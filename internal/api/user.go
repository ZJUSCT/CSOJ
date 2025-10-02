package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/ZJUSCT/CSOJ/internal/auth"
	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/judger"
	"github.com/ZJUSCT/CSOJ/internal/pubsub"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gorilla/websocket"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	contests map[string]*judger.Contest,
	problems map[string]*judger.Problem) *gin.Engine {

	r := gin.Default()

	authHandler := auth.NewGitLabHandler(cfg, db)

	v1 := r.Group("/api/v1")
	{
		// Auth
		authGroup := v1.Group("/auth")
		{
			authGroup.GET("/gitlab/login", authHandler.Login)
			authGroup.GET("/gitlab/callback", authHandler.Callback)
		}

		// Websocket for logs with authorization
		v1.GET("/ws/submissions/:id/logs", func(c *gin.Context) {
			handleWs(c, cfg, db, problems)
		})

		// Publicly accessible info
		v1.GET("/contests", func(c *gin.Context) {
			util.Success(c, contests, "Contests loaded")
		})
		v1.GET("/contests/:id", func(c *gin.Context) {
			contestID := c.Param("id")
			if contest, ok := contests[contestID]; ok {
				util.Success(c, contest, "Contest found")
			} else {
				util.Error(c, http.StatusNotFound, fmt.Errorf("contest not found"))
			}
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
			if problem, ok := problems[problemID]; ok {
				util.Success(c, problem, "Problem found")
			} else {
				util.Error(c, http.StatusNotFound, fmt.Errorf("problem not found"))
			}
		})

		// Authenticated routes
		authed := v1.Group("/")
		authed.Use(AuthMiddleware(cfg.Auth.JWT.Secret))
		{
			// User Profile
			authed.GET("/user/profile", func(c *gin.Context) {
				gitlabID := c.GetString("userID")
				user, err := database.GetUserByGitLabID(db, gitlabID)
				if err != nil {
					util.Error(c, http.StatusNotFound, err)
					return
				}
				util.Success(c, user, "ok")
			})

			authed.PATCH("/user/profile", func(c *gin.Context) {
				gitlabID := c.GetString("userID")
				user, err := database.GetUserByGitLabID(db, gitlabID)
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
				gitlabID := c.GetString("userID")
				user, err := database.GetUserByGitLabID(db, gitlabID)
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

				user.AvatarURL = "/avatars/" + avatarFilename // URL path
				if err := database.UpdateUser(db, user); err != nil {
					util.Error(c, http.StatusInternalServerError, err)
					return
				}
				util.Success(c, user, "Avatar updated")
			})

			// Contest
			authed.POST("/contests/:id/register", func(c *gin.Context) {
				gitlabID := c.GetString("userID")
				contestID := c.Param("id")

				user, err := database.GetUserByGitLabID(db, gitlabID)
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
				gitlabID := c.GetString("userID")
				problemID := c.Param("id")

				user, err := database.GetUserByGitLabID(db, gitlabID)
				if err != nil {
					util.Error(c, http.StatusNotFound, err)
					return
				}

				problem, ok := problems[problemID]
				if !ok {
					util.Error(c, http.StatusNotFound, fmt.Errorf("problem not found"))
					return
				}

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
				gitlabID := c.GetString("userID")
				user, err := database.GetUserByGitLabID(db, gitlabID)
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
				gitlabID := c.GetString("userID")
				user, err := database.GetUserByGitLabID(db, gitlabID)
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

			authed.GET("/submissions/:subID/containers/:conID/log", func(c *gin.Context) {
				subID := c.Param("subID")
				conID := c.Param("conID")
				gitlabID := c.GetString("userID")

				user, err := database.GetUserByGitLabID(db, gitlabID)
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

				problem, ok := problems[sub.ProblemID]
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
		}
	}
	return r
}

func handleWs(c *gin.Context, cfg *config.Config, db *gorm.DB, problems map[string]*judger.Problem) {
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
	gitlabID := claims.Subject
	user, err := database.GetUserByGitLabID(db, gitlabID)
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

	problem, ok := problems[sub.ProblemID]
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
