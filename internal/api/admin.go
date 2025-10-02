package api

import (
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
	contests map[string]*judger.Contest,
	problems map[string]*judger.Problem) *gin.Engine {

	r := gin.Default()

	r.GET("/ws/submissions/:id/logs", func(c *gin.Context) {
		handleAdminWs(c, db)
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
	r.GET("/submissions/:subID/containers/:conID/log", func(c *gin.Context) {
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

		problem, ok := problems[newSub.ProblemID]
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

		if err := database.UpdateSubmissionValidity(db, subID, reqBody.IsValid); err != nil {
			util.Error(c, http.StatusInternalServerError, err)
			return
		}
		util.Success(c, nil, fmt.Sprintf("Submission marked as %v", reqBody.IsValid))
	})

	// Cluster Management
	r.GET("/clusters/status", func(c *gin.Context) {
		status := scheduler.GetClusterStates()
		util.Success(c, status, "Cluster status retrieved")
	})

	return r
}

func handleAdminWs(c *gin.Context, db *gorm.DB) {
	submissionID := c.Param("id")

	_, err := database.GetSubmission(db, submissionID)
	if err != nil {
		c.String(http.StatusNotFound, "submission not found")
		return
	}

	msgChan, unsubscribe := pubsub.GetBroker().Subscribe(submissionID)
	defer unsubscribe()

	conn, err := adminUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		zap.S().Errorf("failed to upgrade admin websocket: %v", err)
		return
	}
	defer conn.Close()

	go func() {
		defer conn.Close()
		for msg := range msgChan {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				zap.S().Warnf("error writing to admin websocket: %v", err)
				break
			}
		}
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				zap.S().Infof("admin websocket unexpected close error: %v", err)
			}
			break
		}
	}
	zap.S().Infof("admin websocket connection closed for submission %s", submissionID)
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
