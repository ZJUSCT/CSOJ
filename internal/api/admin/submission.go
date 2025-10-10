package admin

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/judger"
	"github.com/ZJUSCT/CSOJ/internal/pubsub"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func (h *Handler) getAllSubmissions(c *gin.Context) {
	subs, err := database.GetAllSubmissions(h.db)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, subs, "ok")
}

func (h *Handler) getSubmission(c *gin.Context) {
	sub, err := database.GetSubmission(h.db, c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}
	util.Success(c, sub, "ok")
}

func (h *Handler) getContainerLog(c *gin.Context) {
	con, err := database.GetContainer(h.db, c.Param("conID"))
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

	c.Header("Content-Type", "application/x-ndjson; charset=utf-8")
	io.Copy(c.Writer, file)
}

func (h *Handler) rejudgeSubmission(c *gin.Context) {
	originalSubID := c.Param("id")
	originalSub, err := database.GetSubmission(h.db, originalSubID)
	if err != nil {
		util.Error(c, http.StatusNotFound, "Original submission not found")
		return
	}

	if err := database.UpdateSubmissionValidity(h.db, originalSub.ID, false); err != nil {
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

	srcDir := filepath.Join(h.cfg.Storage.SubmissionContent, originalSub.ID)
	destDir := filepath.Join(h.cfg.Storage.SubmissionContent, newSubID)
	if err := copyDir(srcDir, destDir); err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to copy submission content: %w", err))
		return
	}

	if err := database.CreateSubmission(h.db, &newSub); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	h.appState.RLock()
	problem, ok := h.appState.Problems[newSub.ProblemID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusInternalServerError, "Problem definition not found for rejudge")
		return
	}
	h.scheduler.Submit(&newSub, problem)

	util.Success(c, gin.H{"new_submission_id": newSubID}, "Rejudge successfully submitted")
}

func (h *Handler) updateSubmissionValidity(c *gin.Context) {
	subID := c.Param("id")
	var reqBody struct {
		IsValid bool `json:"is_valid"`
	}
	if err := c.ShouldBindJSON(&reqBody); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	// Get submission details BEFORE updating validity
	sub, err := database.GetSubmission(h.db, subID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}

	if err := database.UpdateSubmissionValidity(h.db, subID, reqBody.IsValid); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	// If a submission is marked as invalid, trigger score recalculation
	if !reqBody.IsValid {
		h.appState.RLock()
		contest, ok := h.appState.ProblemToContestMap[sub.ProblemID]
		h.appState.RUnlock()
		if !ok {
			// This should not happen in a consistent system, but handle it
			zap.S().Errorf("failed to find parent contest for problem %s during score recalculation for submission %s", sub.ProblemID, sub.ID)
		} else {
			if err := database.RecalculateScoresForUserProblem(h.db, sub.UserID, sub.ProblemID, contest.ID, sub.ID); err != nil {
				util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to recalculate scores: %w", err))
				return
			}
		}
	}
	util.Success(c, nil, fmt.Sprintf("Submission marked as %v and scores updated if necessary", reqBody.IsValid))
}

func (h *Handler) interruptSubmission(c *gin.Context) {
	subID := c.Param("id")
	sub, err := database.GetSubmission(h.db, subID)
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
		if err := database.UpdateSubmission(h.db, sub); err != nil {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update submission status: %w", err))
			return
		}
		msg := pubsub.FormatMessage("error", "Submission interrupted by admin.")
		pubsub.GetBroker().Publish(sub.ID, msg)
		pubsub.GetBroker().CloseTopic(sub.ID)
		util.Success(c, nil, "Queued submission interrupted")

	case models.StatusRunning:
		h.appState.RLock()
		problem, ok := h.appState.Problems[sub.ProblemID]
		h.appState.RUnlock()
		if !ok {
			util.Error(c, http.StatusInternalServerError, "Problem definition not found for running submission")
			return
		}

		var nodeDockerHost string
		for _, clusterCfg := range h.cfg.Cluster {
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

		err := h.db.Transaction(func(tx *gorm.DB) error {
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

		// Parse allocated cores from submission record to release them
		var coresToRelease []int
		if sub.AllocatedCores != "" {
			coreStrs := strings.Split(sub.AllocatedCores, ",")
			for _, s := range coreStrs {
				coreID, err := strconv.Atoi(s)
				if err == nil {
					coresToRelease = append(coresToRelease, coreID)
				}
			}
		}
		h.scheduler.ReleaseResources(problem.Cluster, sub.Node, coresToRelease, problem.Memory)

		msg := pubsub.FormatMessage("error", "Submission interrupted by admin.")
		pubsub.GetBroker().Publish(sub.ID, msg)
		pubsub.GetBroker().CloseTopic(sub.ID)
		util.Success(c, nil, "Running submission interrupted successfully")

	case models.StatusSuccess, models.StatusFailed:
		util.Error(c, http.StatusBadRequest, "Submission has already finished and cannot be interrupted")

	default:
		util.Error(c, http.StatusInternalServerError, fmt.Sprintf("Unknown submission status: %s", sub.Status))
	}
}
