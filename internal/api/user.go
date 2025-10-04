package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/auth"
	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/judger"
	"github.com/ZJUSCT/CSOJ/internal/pubsub"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: implement a proper origin check in production
		return true
	},
}

func NewUserRouter(
	cfg *config.Config,
	db *gorm.DB,
	scheduler *judger.Scheduler,
	appState *judger.AppState) *gin.Engine {

	r := gin.Default()

	gitlabAuthHandler := auth.NewGitLabHandler(cfg, db)

	v1 := r.Group("/api/v1")
	{
		// Auth
		authGroup := v1.Group("/auth")
		{
			// GitLab Auth
			gitlabGroup := authGroup.Group("/gitlab")
			gitlabGroup.GET("/login", gitlabAuthHandler.Login)
			gitlabGroup.GET("/callback", gitlabAuthHandler.Callback)

			// Local Username/Password Auth (if enabled)
			if cfg.Auth.Local.Enabled {
				localAuthGroup := authGroup.Group("/local")
				{
					localAuthGroup.POST("/register", func(c *gin.Context) {
						var req struct {
							Username string `json:"username" binding:"required"`
							Password string `json:"password" binding:"required"`
							Nickname string `json:"nickname"`
						}
						if err := c.ShouldBindJSON(&req); err != nil {
							util.Error(c, http.StatusBadRequest, err)
							return
						}

						_, err := database.GetUserByUsername(db, req.Username)
						if !errors.Is(err, gorm.ErrRecordNotFound) {
							if err == nil {
								util.Error(c, http.StatusConflict, "username already exists")
							} else {
								util.Error(c, http.StatusInternalServerError, "database error")
							}
							return
						}

						hashedPassword, err := auth.HashPassword(req.Password)
						if err != nil {
							util.Error(c, http.StatusInternalServerError, "failed to hash password")
							return
						}

						newUser := models.User{
							ID:           uuid.NewString(),
							Username:     req.Username,
							PasswordHash: hashedPassword,
							Nickname:     req.Nickname,
						}
						if newUser.Nickname == "" {
							newUser.Nickname = newUser.Username
						}

						if err := database.CreateUser(db, &newUser); err != nil {
							util.Error(c, http.StatusInternalServerError, "failed to create user")
							return
						}

						zap.S().Infof("new local user registered: %s", newUser.Username)
						util.Success(c, gin.H{"id": newUser.ID, "username": newUser.Username}, "User registered successfully")
					})

					localAuthGroup.POST("/login", func(c *gin.Context) {
						var req struct {
							Username string `json:"username" binding:"required"`
							Password string `json:"password" binding:"required"`
						}
						if err := c.ShouldBindJSON(&req); err != nil {
							util.Error(c, http.StatusBadRequest, err)
							return
						}

						user, err := database.GetUserByUsername(db, req.Username)
						if err != nil {
							if errors.Is(err, gorm.ErrRecordNotFound) {
								util.Error(c, http.StatusUnauthorized, "invalid username or password")
							} else {
								util.Error(c, http.StatusInternalServerError, "database error")
							}
							return
						}

						if user.PasswordHash == "" {
							util.Error(c, http.StatusUnauthorized, "user registered via GitLab, please use GitLab login")
							return
						}

						if !auth.CheckPasswordHash(req.Password, user.PasswordHash) {
							util.Error(c, http.StatusUnauthorized, "invalid username or password")
							return
						}

						jwtToken, err := auth.GenerateJWT(user.ID, cfg.Auth.JWT.Secret, cfg.Auth.JWT.ExpireHours)
						if err != nil {
							util.Error(c, http.StatusInternalServerError, "failed to generate JWT")
							return
						}
						util.Success(c, gin.H{"token": jwtToken}, "Login successful")
					})
				}
			}
		}

		// Websocket for logs with authorization
		v1.GET("/ws/submissions/:id/logs", func(c *gin.Context) {
			handleWs(c, cfg, db, appState)
		})

		// Publicly accessible info
		v1.GET("/contests", func(c *gin.Context) {
			appState.RLock()
			defer appState.RUnlock()
			util.Success(c, appState.Contests, "Contests loaded")
		})
		v1.GET("/contests/:id", func(c *gin.Context) {
			contestID := c.Param("id")
			appState.RLock()
			contest, ok := appState.Contests[contestID]
			appState.RUnlock()

			if !ok {
				util.Error(c, http.StatusNotFound, fmt.Errorf("contest not found"))
				return
			}

			now := time.Now()
			// For contests that haven't started or have already ended, hide the problem list.
			if now.Before(contest.StartTime) || now.After(contest.EndTime) {
				// Create a copy to avoid modifying the original map entry
				contestCopy := *contest
				contestCopy.ProblemIDs = []string{} // Empty the problem list
				util.Success(c, contestCopy, "Contest found, but is not currently active")
				return
			}
			util.Success(c, contest, "Contest found")
		})
		v1.GET("/contests/:id/leaderboard", func(c *gin.Context) {
			contestID := c.Param("id")
			leaderboard, err := database.GetLeaderboard(db, contestID)
			if err != nil {
				util.Error(c, http.StatusInternalServerError, err)
				return
			}
			util.Success(c, leaderboard, "Leaderboard retrieved")
		})
		v1.GET("/problems/:id", func(c *gin.Context) {
			problemID := c.Param("id")
			appState.RLock()
			problem, ok := appState.Problems[problemID]
			if ok {
				parentContest, parentOk := appState.ProblemToContestMap[problemID]
				ok = parentOk
				if ok {
					now := time.Now()
					// Check if the contest and problem are active
					if now.Before(parentContest.StartTime) {
						util.Error(c, http.StatusForbidden, fmt.Errorf("contest has not started yet"))
						appState.RUnlock()
						return
					}
					if now.Before(problem.StartTime) {
						util.Error(c, http.StatusForbidden, fmt.Errorf("problem has not started yet"))
						appState.RUnlock()
						return
					}
				} else {
					util.Error(c, http.StatusInternalServerError, fmt.Errorf("internal server error: problem has no parent contest"))
					appState.RUnlock()
					return
				}
			}
			appState.RUnlock()

			if !ok {
				util.Error(c, http.StatusNotFound, fmt.Errorf("problem not found"))
				return
			}

			util.Success(c, problem, "Problem found")
		})

		// Authenticated routes
		authed := v1.Group("/")
		authed.Use(AuthMiddleware(cfg.Auth.JWT.Secret))
		{
			// User Profile
			authed.GET("/user/profile", func(c *gin.Context) {
				userID := c.GetString("userID")
				user, err := database.GetUserByID(db, userID)
				if err != nil {
					util.Error(c, http.StatusNotFound, err)
					return
				}
				// Prepend API path to avatar filename if it's not a full URL
				if user.AvatarURL != "" && !strings.HasPrefix(user.AvatarURL, "http") {
					user.AvatarURL = fmt.Sprintf("/api/v1/assets/avatars/%s", user.AvatarURL)
				}
				util.Success(c, user, "ok")
			})

			authed.PATCH("/user/profile", func(c *gin.Context) {
				userID := c.GetString("userID")
				user, err := database.GetUserByID(db, userID)
				if err != nil {
					util.Error(c, http.StatusNotFound, err)
					return
				}
				var reqBody struct {
					Nickname  string `json:"nickname"`
					Signature string `json:"signature"`
				}
				if err := c.ShouldBindJSON(&reqBody); err != nil {
					util.Error(c, http.StatusBadRequest, err)
					return
				}
				user.Nickname = reqBody.Nickname
				user.Signature = reqBody.Signature
				if err := database.UpdateUser(db, user); err != nil {
					util.Error(c, http.StatusInternalServerError, err)
					return
				}
				util.Success(c, user, "Profile updated")
			})

			authed.POST("/user/avatar", func(c *gin.Context) {
				userID := c.GetString("userID")
				user, err := database.GetUserByID(db, userID)
				if err != nil {
					util.Error(c, http.StatusNotFound, err)
					return
				}

				file, err := c.FormFile("avatar")
				if err != nil {
					util.Error(c, http.StatusBadRequest, "Avatar file not provided")
					return
				}

				ext := filepath.Ext(file.Filename)
				avatarFilename := fmt.Sprintf("%s%s", user.ID, ext)
				avatarPath := filepath.Join(cfg.Storage.UserAvatar, avatarFilename)

				if err := c.SaveUploadedFile(file, avatarPath); err != nil {
					util.Error(c, http.StatusInternalServerError, "Failed to save avatar")
					return
				}

				user.AvatarURL = avatarFilename // Store only the filename
				if err := database.UpdateUser(db, user); err != nil {
					util.Error(c, http.StatusInternalServerError, err)
					return
				}
				util.Success(c, user, "Avatar updated")
			})

			// Contest
			authed.POST("/contests/:id/register", func(c *gin.Context) {
				userID := c.GetString("userID")
				contestID := c.Param("id")

				appState.RLock()
				contest, ok := appState.Contests[contestID]
				appState.RUnlock()

				if !ok {
					util.Error(c, http.StatusNotFound, fmt.Errorf("contest not found"))
					return
				}

				now := time.Now()
				if now.Before(contest.StartTime) {
					util.Error(c, http.StatusForbidden, fmt.Errorf("contest has not started, cannot register"))
					return
				}
				if now.After(contest.EndTime) {
					util.Error(c, http.StatusForbidden, fmt.Errorf("contest has ended, cannot register"))
					return
				}

				user, err := database.GetUserByID(db, userID)
				if err != nil {
					util.Error(c, http.StatusNotFound, err)
					return
				}

				if err := database.RegisterForContest(db, user.ID, contestID); err != nil {
					if err.Error() == "already registered" {
						util.Error(c, http.StatusConflict, err)
						return
					}
					util.Error(c, http.StatusInternalServerError, err)
					return
				}
				util.Success(c, nil, "Successfully registered for contest")
			})

			// Submissions
			authed.POST("/problems/:id/submit", func(c *gin.Context) {
				userID := c.GetString("userID")
				problemID := c.Param("id")

				user, err := database.GetUserByID(db, userID)
				if err != nil {
					util.Error(c, http.StatusNotFound, err)
					return
				}

				appState.RLock()
				problem, ok := appState.Problems[problemID]
				if !ok {
					appState.RUnlock()
					util.Error(c, http.StatusNotFound, fmt.Errorf("problem not found"))
					return
				}

				parentContest, ok := appState.ProblemToContestMap[problemID]
				if !ok {
					appState.RUnlock()
					util.Error(c, http.StatusInternalServerError, fmt.Errorf("internal server error: problem has no parent contest"))
					return
				}

				// Check time restrictions for submission
				now := time.Now()
				if now.Before(parentContest.StartTime) || now.After(parentContest.EndTime) {
					appState.RUnlock()
					util.Error(c, http.StatusForbidden, fmt.Errorf("cannot submit because the contest is not active"))
					return
				}
				if now.Before(problem.StartTime) || now.After(problem.EndTime) {
					appState.RUnlock()
					util.Error(c, http.StatusForbidden, fmt.Errorf("cannot submit because the problem is not active"))
					return
				}
				appState.RUnlock()

				form, err := c.MultipartForm()
				if err != nil {
					util.Error(c, http.StatusBadRequest, err)
					return
				}
				files := form.File["files"]

				submissionID := uuid.New().String()
				submissionPath := filepath.Join(cfg.Storage.SubmissionContent, submissionID)
				if err := os.MkdirAll(submissionPath, 0755); err != nil {
					util.Error(c, http.StatusInternalServerError, err)
					return
				}

				for _, file := range files {
					dst := filepath.Join(submissionPath, file.Filename)
					if err := c.SaveUploadedFile(file, dst); err != nil {
						util.Error(c, http.StatusInternalServerError, err)
						return
					}
				}

				sub := models.Submission{
					ID:        submissionID,
					ProblemID: problemID,
					UserID:    user.ID,
					Status:    models.StatusQueued,
					Cluster:   problem.Cluster,
					IsValid:   true,
				}

				if err := database.CreateSubmission(db, &sub); err != nil {
					util.Error(c, http.StatusInternalServerError, err)
					return
				}

				scheduler.Submit(&sub, problem)
				util.Success(c, gin.H{"submission_id": submissionID}, "Submission received")
			})

			authed.GET("/submissions", func(c *gin.Context) {
				userID := c.GetString("userID")
				user, err := database.GetUserByID(db, userID)
				if err != nil {
					util.Error(c, http.StatusNotFound, err)
					return
				}
				subs, err := database.GetSubmissionsByUserID(db, user.ID)
				if err != nil {
					util.Error(c, http.StatusInternalServerError, err)
					return
				}
				util.Success(c, subs, "ok")
			})

			authed.GET("/submissions/:id", func(c *gin.Context) {
				subID := c.Param("id")
				userID := c.GetString("userID")
				user, err := database.GetUserByID(db, userID)
				if err != nil {
					util.Error(c, http.StatusNotFound, err)
					return
				}
				sub, err := database.GetSubmission(db, subID)
				if err != nil {
					util.Error(c, http.StatusNotFound, err)
					return
				}
				if sub.UserID != user.ID {
					util.Error(c, http.StatusForbidden, fmt.Errorf("you can only view your own submissions"))
					return
				}
				util.Success(c, sub, "ok")
			})

			authed.POST("/submissions/:id/interrupt", func(c *gin.Context) {
				subID := c.Param("id")
				userID := c.GetString("userID")
				user, err := database.GetUserByID(db, userID)
				if err != nil {
					util.Error(c, http.StatusNotFound, err)
					return
				}

				sub, err := database.GetSubmission(db, subID)
				if err != nil {
					if err == gorm.ErrRecordNotFound {
						util.Error(c, http.StatusNotFound, "Submission not found")
						return
					}
					util.Error(c, http.StatusInternalServerError, err)
					return
				}

				// Authorization check
				if sub.UserID != user.ID {
					util.Error(c, http.StatusForbidden, "You can only interrupt your own submissions")
					return
				}

				switch sub.Status {
				case models.StatusQueued:
					sub.Status = models.StatusFailed
					sub.Info = models.JSONMap{"error": "Interrupted by user while in queue"}
					if err := database.UpdateSubmission(db, sub); err != nil {
						util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update submission status: %w", err))
						return
					}
					msg := pubsub.FormatMessage("error", "Submission interrupted by user.")
					pubsub.GetBroker().Publish(subID, msg)
					pubsub.GetBroker().CloseTopic(subID)
					util.Success(c, nil, "Queued submission interrupted")

				case models.StatusRunning:
					appState.RLock()
					problem, ok := appState.Problems[sub.ProblemID]
					appState.RUnlock()
					if !ok {
						util.Error(c, http.StatusInternalServerError, "Problem definition not found for running submission")
						return
					}

					var nodeDockerHost string
					for _, clusterCfg := range cfg.Cluster {
						if clusterCfg.Name == sub.Cluster {
							for _, nodeCfg := range clusterCfg.Nodes {
								if nodeCfg.Name == sub.Node {
									nodeDockerHost = nodeCfg.Docker
									break
								}
							}
							break
						}
					}

					if nodeDockerHost == "" {
						zap.S().Errorf("node config '%s'/'%s' not found for sub %s, cannot stop container but will mark as failed", sub.Cluster, sub.Node, sub.ID)
					} else {
						docker, err := judger.NewDockerManager(nodeDockerHost)
						if err != nil {
							util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to connect to docker on node %s: %w", sub.Node, err))
							return
						}
						for _, container := range sub.Containers {
							if container.DockerID != "" {
								zap.S().Infof("forcefully cleaning up container %s for submission %s", container.DockerID, sub.ID)
								docker.CleanupContainer(container.DockerID)
							}
						}
					}

					err := db.Transaction(func(tx *gorm.DB) error {
						if err := tx.Model(&models.Submission{}).Where("id = ?", subID).Updates(map[string]interface{}{
							"status": models.StatusFailed,
							"info":   models.JSONMap{"error": "Interrupted by user while running"},
						}).Error; err != nil {
							return err
						}
						return tx.Model(&models.Container{}).Where("submission_id = ? AND status = ?", subID, models.StatusRunning).Update("status", models.StatusFailed).Error
					})
					if err != nil {
						util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update database: %w", err))
						return
					}

					scheduler.ReleaseResources(problem.Cluster, sub.Node, problem.CPU, problem.Memory)

					msg := pubsub.FormatMessage("error", "Submission interrupted by user.")
					pubsub.GetBroker().Publish(subID, msg)
					pubsub.GetBroker().CloseTopic(subID)
					util.Success(c, nil, "Running submission interrupted successfully")

				case models.StatusSuccess, models.StatusFailed:
					util.Error(c, http.StatusBadRequest, "Submission has already finished and cannot be interrupted")

				default:
					util.Error(c, http.StatusInternalServerError, fmt.Sprintf("Unknown submission status: %s", sub.Status))
				}
			})

			authed.GET("/submissions/:id/queue_position", func(c *gin.Context) {
				subID := c.Param("id")
				userID := c.GetString("userID")

				user, err := database.GetUserByID(db, userID)
				if err != nil {
					util.Error(c, http.StatusNotFound, err)
					return
				}

				sub, err := database.GetSubmission(db, subID)
				if err != nil {
					util.Error(c, http.StatusNotFound, err)
					return
				}

				if sub.UserID != user.ID {
					util.Error(c, http.StatusForbidden, fmt.Errorf("you can only view your own submissions"))
					return
				}

				if sub.Status != models.StatusQueued {
					util.Success(c, gin.H{"position": 0}, "Submission is not in queue")
					return
				}

				count, err := database.CountQueuedSubmissionsBefore(db, sub.Cluster, sub.CreatedAt)
				if err != nil {
					util.Error(c, http.StatusInternalServerError, err)
					return
				}

				util.Success(c, gin.H{"position": count}, "Queue position retrieved successfully")
			})

			authed.GET("/submissions/:id/containers/:conID/log", func(c *gin.Context) {
				subID := c.Param("id")
				conID := c.Param("conID")
				userID := c.GetString("userID")

				user, err := database.GetUserByID(db, userID)
				if err != nil {
					util.Error(c, http.StatusNotFound, "user not found")
					return
				}

				sub, err := database.GetSubmission(db, subID)
				if err != nil {
					util.Error(c, http.StatusNotFound, "submission not found")
					return
				}

				// Authorization Check : Ownership
				if sub.UserID != user.ID {
					util.Error(c, http.StatusForbidden, "you can only view your own submissions")
					return
				}

				var targetContainer *models.Container
				var containerIndex = -1
				// Sort containers by creation time to determine their step index
				sort.Slice(sub.Containers, func(i, j int) bool {
					return sub.Containers[i].CreatedAt.Before(sub.Containers[j].CreatedAt)
				})
				for i, c := range sub.Containers {
					if c.ID == conID {
						targetContainer = &sub.Containers[i]
						containerIndex = i
						break
					}
				}

				if targetContainer == nil {
					util.Error(c, http.StatusNotFound, "container not found in this submission")
					return
				}

				appState.RLock()
				problem, ok := appState.Problems[sub.ProblemID]
				appState.RUnlock()
				if !ok {
					util.Error(c, http.StatusInternalServerError, "problem definition not found")
					return
				}

				// Authorization Check : `show` flag in problem.yaml
				if containerIndex >= len(problem.Workflow) || !problem.Workflow[containerIndex].Show {
					util.Error(c, http.StatusForbidden, "you are not allowed to view the log for this step")
					return
				}

				file, err := os.Open(targetContainer.LogFilePath)
				if err != nil {
					if os.IsNotExist(err) {
						util.Error(c, http.StatusNotFound, "log file not found on disk")
						return
					}
					util.Error(c, http.StatusInternalServerError, "failed to open log file")
					return
				}
				defer file.Close()

				c.Header("Content-Type", "text/plain; charset=utf-8")
				io.Copy(c.Writer, file)
			})

			assets := authed.Group("/assets")
			{
				assets.GET("/avatars/:filename", func(c *gin.Context) {
					filename := c.Param("filename")
					// Basic security: prevent path traversal
					cleanFilename := filepath.Base(filename)
					if cleanFilename != filename {
						util.Error(c, http.StatusBadRequest, "invalid filename")
						return
					}

					fullPath := filepath.Join(cfg.Storage.UserAvatar, cleanFilename)

					if _, err := os.Stat(fullPath); os.IsNotExist(err) {
						util.Error(c, http.StatusNotFound, "avatar not found")
						return
					}
					c.File(fullPath)
				})

				assets.GET("/contests/:id/*assetpath", func(c *gin.Context) {
					contestID := c.Param("id")
					assetPath := c.Param("assetpath")

					appState.RLock()
					contest, ok := appState.Contests[contestID]
					appState.RUnlock()
					if !ok {
						util.Error(c, http.StatusNotFound, "contest not found")
						return
					}

					// Security: ensure the requested path is within the allowed assets directory
					baseAssetDir := filepath.Join(contest.BasePath, "index.assets")
					requestedFile := filepath.Join(baseAssetDir, assetPath)

					safeBase, err := filepath.Abs(baseAssetDir)
					if err != nil {
						util.Error(c, http.StatusInternalServerError, "internal server error")
						return
					}
					safeRequested, err := filepath.Abs(requestedFile)
					if err != nil {
						util.Error(c, http.StatusInternalServerError, "internal server error")
						return
					}

					if !strings.HasPrefix(safeRequested, safeBase) {
						util.Error(c, http.StatusForbidden, "access denied")
						return
					}

					if _, err := os.Stat(safeRequested); os.IsNotExist(err) {
						util.Error(c, http.StatusNotFound, "asset not found")
						return
					}
					c.File(safeRequested)
				})

				assets.GET("/problems/:id/*assetpath", func(c *gin.Context) {
					problemID := c.Param("id")
					assetPath := c.Param("assetpath")

					appState.RLock()
					problem, ok := appState.Problems[problemID]
					if !ok {
						appState.RUnlock()
						util.Error(c, http.StatusNotFound, "problem not found")
						return
					}

					// --- Authorization Logic (same as GET /problems/:id) ---
					parentContest, ok := appState.ProblemToContestMap[problemID]
					if !ok {
						appState.RUnlock()
						util.Error(c, http.StatusInternalServerError, "internal server error: problem has no parent contest")
						return
					}
					now := time.Now()
					if now.Before(parentContest.StartTime) {
						appState.RUnlock()
						util.Error(c, http.StatusForbidden, "contest has not started yet")
						return
					}
					if now.Before(problem.StartTime) {
						appState.RUnlock()
						util.Error(c, http.StatusForbidden, "problem has not started yet")
						return
					}
					appState.RUnlock()
					// --- End Authorization ---

					// --- Security Logic (same as contest assets) ---
					baseAssetDir := filepath.Join(problem.BasePath, "index.assets")
					requestedFile := filepath.Join(baseAssetDir, assetPath)

					safeBase, err := filepath.Abs(baseAssetDir)
					if err != nil {
						util.Error(c, http.StatusInternalServerError, "internal server error")
						return
					}
					safeRequested, err := filepath.Abs(requestedFile)
					if err != nil {
						util.Error(c, http.StatusInternalServerError, "internal server error")
						return
					}

					if !strings.HasPrefix(safeRequested, safeBase) {
						util.Error(c, http.StatusForbidden, "access denied")
						return
					}

					if _, err := os.Stat(safeRequested); os.IsNotExist(err) {
						util.Error(c, http.StatusNotFound, "asset not found")
						return
					}
					c.File(safeRequested)
				})
			}
		}
	}
	return r
}

func handleWs(c *gin.Context, cfg *config.Config, db *gorm.DB, appState *judger.AppState) {
	submissionID := c.Param("id")
	tokenString := c.Query("token")

	if tokenString == "" {
		c.String(http.StatusUnauthorized, "token query parameter is required")
		return
	}

	claims, err := auth.ValidateJWT(tokenString, cfg.Auth.JWT.Secret)
	if err != nil {
		c.String(http.StatusUnauthorized, "invalid token")
		return
	}
	userID := claims.Subject
	user, err := database.GetUserByID(db, userID)
	if err != nil {
		c.String(http.StatusNotFound, "user not found")
		return
	}

	sub, err := database.GetSubmission(db, submissionID)
	if err != nil {
		c.String(http.StatusNotFound, "submission not found")
		return
	}

	// Authorization Check : Ownership
	if sub.UserID != user.ID {
		c.String(http.StatusForbidden, "you can only view your own submissions")
		return
	}

	appState.RLock()
	problem, ok := appState.Problems[sub.ProblemID]
	appState.RUnlock()
	if !ok {
		c.String(http.StatusInternalServerError, "problem definition not found")
		return
	}

	// Authorization Check : `show` flag in problem.yaml
	for _, step := range problem.Workflow {
		if !step.Show {
			c.String(http.StatusForbidden, "live stream is not available for this problem because it contains hidden steps")
			return
		}
	}

	msgChan, unsubscribe := pubsub.GetBroker().Subscribe(submissionID)
	defer unsubscribe()

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		zap.S().Errorf("failed to upgrade websocket: %v", err)
		return
	}
	defer conn.Close()

	go func() {
		defer conn.Close()
		for msg := range msgChan {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				zap.S().Warnf("error writing to websocket: %v", err)
				break
			}
		}
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				zap.S().Infof("websocket unexpected close error: %v", err)
			}
			break
		}
	}
	zap.S().Infof("websocket connection closed for submission %s", submissionID)
}
