package api

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/judger"
	"github.com/ZJUSCT/CSOJ/internal/pubsub"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/gin-gonic/gin"
)

var adminUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func NewAdminRouter(
	cfg *config.Config,
	db *gorm.DB,
	scheduler *judger.Scheduler,
	appState *judger.AppState) *gin.Engine {

	r := gin.Default()

	r.Use(CORSMiddleware(cfg.CORS))

	r.GET("/ws/submissions/:id/containers/:conID/logs", func(c *gin.Context) {
		handleAdminContainerWs(c, db)
	})

	// Management
	r.POST("/reload", func(c *gin.Context) {
		// Load new data into temporary variables
		zap.S().Info("starting reload process...")
		newContests, newProblems, err := judger.LoadAllContestsAndProblems(cfg.Contest)
		if err != nil {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to load new contests/problems: %w", err))
			return
		}
		zap.S().Infof("successfully loaded %d new contests and %d new problems from disk", len(newContests), len(newProblems))

		newProblemIDs := make(map[string]struct{}, len(newProblems))
		for id := range newProblems {
			newProblemIDs[id] = struct{}{}
		}

		// Find submissions whose problems have been deleted
		var allSubmissions []models.Submission
		// Fetch submissions with their containers to handle running ones
		if err := db.Preload("Containers").Find(&allSubmissions).Error; err != nil {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to get all submissions: %w", err))
			return
		}

		// Process and delete submissions for removed problems
		deletedCount := 0
		for _, sub := range allSubmissions {
			if _, exists := newProblemIDs[sub.ProblemID]; !exists {
				// This submission's problem was removed. Time to delete the submission.
				zap.S().Infof("problem %s for submission %s was removed, preparing to delete submission", sub.ProblemID, sub.ID)

				// For running submissions, we must clean up the container and resources first.
				if sub.Status == models.StatusRunning {
					appState.RLock()
					problem, ok := appState.Problems[sub.ProblemID]
					appState.RUnlock()

					if !ok {
						zap.S().Warnf("problem definition for running submission %s (problem %s) not found in old state during reload. Cannot stop container or release resources cleanly. DB record will be deleted anyway.", sub.ID, sub.ProblemID)
					} else {
						// This logic is adapted from the interrupt handler.
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
							zap.S().Errorf("node config '%s'/'%s' not found for sub %s, cannot stop container", sub.Cluster, sub.Node, sub.ID)
						} else {
							docker, err := judger.NewDockerManager(nodeDockerHost)
							if err != nil {
								zap.S().Errorf("failed to connect to docker on node %s for sub %s cleanup: %v", sub.Node, sub.ID, err)
							} else {
								for _, container := range sub.Containers {
									if container.DockerID != "" {
										zap.S().Infof("forcefully cleaning up container %s for deleted submission %s", container.DockerID, sub.ID)
										docker.CleanupContainer(container.DockerID)
									}
								}
							}
						}
						scheduler.ReleaseResources(problem.Cluster, sub.Node, problem.CPU, problem.Memory)
					}
				}

				// Hard delete the submission from the database.
				if err := db.Delete(&models.Submission{}, sub.ID).Error; err != nil {
					zap.S().Errorf("failed to delete submission %s during reload: %v", sub.ID, err)
				} else {
					deletedCount++
					zap.S().Infof("deleted submission %s because its problem %s was removed", sub.ID, sub.ProblemID)
				}
			}
		}

		// Create new Problem-to-Contest map
		newProblemToContestMap := make(map[string]*judger.Contest)
		for _, contest := range newContests {
			for _, problemID := range contest.ProblemIDs {
				newProblemToContestMap[problemID] = contest
			}
		}

		// Atomically update the shared state
		appState.Lock()
		appState.Contests = newContests
		appState.Problems = newProblems
		appState.ProblemToContestMap = newProblemToContestMap
		appState.Unlock()
		zap.S().Info("app state reloaded successfully")

		util.Success(c, gin.H{
			"contests_loaded":     len(newContests),
			"problems_loaded":     len(newProblems),
			"submissions_deleted": deletedCount,
		}, "Reload successful")
	})

	// User Management
	r.GET("/users", func(c *gin.Context) {
		users, err := database.GetAllUsers(db)
		if err != nil {
			util.Error(c, http.StatusInternalServerError, err)
			return
		}
		util.Success(c, users, "Users retrieved successfully")
	})
	r.POST("/users", func(c *gin.Context) {
		var user models.User
		if err := c.ShouldBindJSON(&user); err != nil {
			util.Error(c, http.StatusBadRequest, err)
			return
		}
		user.ID = uuid.NewString()
		if err := database.CreateUser(db, &user); err != nil {
			util.Error(c, http.StatusInternalServerError, err)
			return
		}
		util.Success(c, user, "User created successfully")
	})
	r.DELETE("/users/:id", func(c *gin.Context) {
		userID := c.Param("id")
		if err := database.DeleteUser(db, userID); err != nil {
			util.Error(c, http.StatusInternalServerError, err)
			return
		}
		util.Success(c, nil, "User deleted successfully")
	})

	// Submission Management
	r.GET("/submissions", func(c *gin.Context) {
		subs, err := database.GetAllSubmissions(db)
		if err != nil {
			util.Error(c, http.StatusInternalServerError, err)
			return
		}
		util.Success(c, subs, "ok")
	})

	r.GET("/submissions/:id", func(c *gin.Context) {
		sub, err := database.GetSubmission(db, c.Param("id"))
		if err != nil {
			util.Error(c, http.StatusNotFound, err)
			return
		}
		util.Success(c, sub, "ok")
	})

	// Get container log
	r.GET("/submissions/:id/containers/:conID/log", func(c *gin.Context) {
		con, err := database.GetContainer(db, c.Param("conID"))
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				util.Error(c, http.StatusNotFound, "Container not found")
				return
			}
			util.Error(c, http.StatusInternalServerError, err)
			return
		}

		if con.LogFilePath == "" {
			util.Error(c, http.StatusNotFound, "Log file path not recorded")
			return
		}

		file, err := os.Open(con.LogFilePath)
		if err != nil {
			if os.IsNotExist(err) {
				util.Error(c, http.StatusNotFound, "Log file not found on disk")
				return
			}
			util.Error(c, http.StatusInternalServerError, "Failed to open log file")
			return
		}
		defer file.Close()

		c.Header("Content-Type", "text/plain; charset=utf-8")
		io.Copy(c.Writer, file)
	})

	r.POST("/submissions/:id/rejudge", func(c *gin.Context) {
		originalSubID := c.Param("id")
		originalSub, err := database.GetSubmission(db, originalSubID)
		if err != nil {
			util.Error(c, http.StatusNotFound, "Original submission not found")
			return
		}

		if err := database.UpdateSubmissionValidity(db, originalSub.ID, false); err != nil {
			util.Error(c, http.StatusInternalServerError, err)
			return
		}

		newSubID := uuid.NewString()
		newSub := models.Submission{
			ID:        newSubID,
			ProblemID: originalSub.ProblemID,
			UserID:    originalSub.UserID,
			Status:    models.StatusQueued,
			Cluster:   originalSub.Cluster,
			IsValid:   true,
		}

		srcDir := filepath.Join(cfg.Storage.SubmissionContent, originalSub.ID)
		destDir := filepath.Join(cfg.Storage.SubmissionContent, newSubID)
		if err := copyDir(srcDir, destDir); err != nil {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to copy submission content: %w", err))
			return
		}

		if err := database.CreateSubmission(db, &newSub); err != nil {
			util.Error(c, http.StatusInternalServerError, err)
			return
		}

		appState.RLock()
		problem, ok := appState.Problems[newSub.ProblemID]
		appState.RUnlock()
		if !ok {
			util.Error(c, http.StatusInternalServerError, "Problem definition not found for rejudge")
			return
		}
		scheduler.Submit(&newSub, problem)

		util.Success(c, gin.H{"new_submission_id": newSubID}, "Rejudge successfully submitted")
	})

	r.PATCH("/submissions/:id/validity", func(c *gin.Context) {
		subID := c.Param("id")
		var reqBody struct {
			IsValid bool `json:"is_valid"`
		}
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			util.Error(c, http.StatusBadRequest, err)
			return
		}

		// Get submission details BEFORE updating validity
		sub, err := database.GetSubmission(db, subID)
		if err != nil {
			util.Error(c, http.StatusNotFound, err)
			return
		}

		if err := database.UpdateSubmissionValidity(db, subID, reqBody.IsValid); err != nil {
			util.Error(c, http.StatusInternalServerError, err)
			return
		}

		// If a submission is marked as invalid, trigger score recalculation
		if !reqBody.IsValid {
			appState.RLock()
			contest, ok := appState.ProblemToContestMap[sub.ProblemID]
			appState.RUnlock()
			if !ok {
				// This should not happen in a consistent system, but handle it
				zap.S().Errorf("failed to find parent contest for problem %s during score recalculation for submission %s", sub.ProblemID, sub.ID)
			} else {
				if err := database.RecalculateScoresForUserProblem(db, sub.UserID, sub.ProblemID, contest.ID, sub.ID); err != nil {
					util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to recalculate scores: %w", err))
					return
				}
			}
		}
		util.Success(c, nil, fmt.Sprintf("Submission marked as %v and scores updated if necessary", reqBody.IsValid))
	})

	r.POST("/submissions/:id/interrupt", func(c *gin.Context) {
		subID := c.Param("id")
		sub, err := database.GetSubmission(db, subID)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				util.Error(c, http.StatusNotFound, "Submission not found")
				return
			}
			util.Error(c, http.StatusInternalServerError, err)
			return
		}

		switch sub.Status {
		case models.StatusQueued:
			sub.Status = models.StatusFailed
			sub.Info = models.JSONMap{"error": "Interrupted by admin while in queue"}
			if err := database.UpdateSubmission(db, sub); err != nil {
				util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update submission status: %w", err))
				return
			}
			msg := pubsub.FormatMessage("error", "Submission interrupted by admin.")
			pubsub.GetBroker().Publish(sub.ID, msg)
			pubsub.GetBroker().CloseTopic(sub.ID)
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
					"info":   models.JSONMap{"error": "Interrupted by admin while running"},
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

			msg := pubsub.FormatMessage("error", "Submission interrupted by admin.")
			pubsub.GetBroker().Publish(sub.ID, msg)
			pubsub.GetBroker().CloseTopic(sub.ID)
			util.Success(c, nil, "Running submission interrupted successfully")

		case models.StatusSuccess, models.StatusFailed:
			util.Error(c, http.StatusBadRequest, "Submission has already finished and cannot be interrupted")

		default:
			util.Error(c, http.StatusInternalServerError, fmt.Sprintf("Unknown submission status: %s", sub.Status))
		}
	})

	// Cluster Management
	r.GET("/clusters/status", func(c *gin.Context) {
		status := scheduler.GetClusterStates()
		util.Success(c, status, "Cluster status retrieved")
	})

	return r
}

func handleAdminContainerWs(c *gin.Context, db *gorm.DB) {
	submissionID := c.Param("id")
	containerID := c.Param("conID")

	con, err := database.GetContainer(db, containerID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.String(http.StatusNotFound, "container not found")
			return
		}
		c.String(http.StatusInternalServerError, "database error")
		return
	}

	if con.SubmissionID != submissionID {
		c.String(http.StatusForbidden, "container does not belong to this submission")
		return
	}

	conn, err := adminUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		zap.S().Errorf("failed to upgrade admin websocket: %v", err)
		return
	}
	defer conn.Close()

	if con.Status == models.StatusRunning {
		// Real-time streaming for a running container
		msgChan, unsubscribe := pubsub.GetBroker().Subscribe(containerID)
		defer unsubscribe()

		// Goroutine to pump messages from pubsub to websocket
		clientClosed := make(chan struct{})
		go func() {
			defer close(clientClosed)
			for msg := range msgChan {
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					zap.S().Warnf("error writing to admin websocket: %v", err)
					return
				}
			}
		}()

		// Read loop to detect client close
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					zap.S().Infof("admin websocket unexpected close error: %v", err)
				}
				break
			}
		}
		<-clientClosed // Wait for writer goroutine to finish before returning

	} else { // StatusSuccess or StatusFailed: Stream the stored log file
		if con.LogFilePath == "" {
			msg := pubsub.FormatMessage("error", "Log file path not recorded for this container.")
			conn.WriteMessage(websocket.TextMessage, msg)
			return
		}

		file, err := os.Open(con.LogFilePath)
		if err != nil {
			if os.IsNotExist(err) {
				msg := pubsub.FormatMessage("error", "Log file not found on disk.")
				conn.WriteMessage(websocket.TextMessage, msg)
			} else {
				msg := pubsub.FormatMessage("error", "Failed to open log file.")
				conn.WriteMessage(websocket.TextMessage, msg)
			}
			return
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			// Replay the raw log file line by line, classifying it as stdout for the client
			msg := pubsub.FormatMessage("stdout", scanner.Text())
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return // Client disconnected
			}
		}

		if err := scanner.Err(); err != nil {
			zap.S().Errorf("error reading log file for container %s: %v", con.ID, err)
		}

		msg := pubsub.FormatMessage("info", "Log stream finished.")
		conn.WriteMessage(websocket.TextMessage, msg)
	}
	zap.S().Infof("admin websocket connection closed for container %s", containerID)
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
