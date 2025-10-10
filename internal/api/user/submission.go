package user

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

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

func (h *Handler) submitToProblem(c *gin.Context) {
	userID := c.GetString("userID")
	problemID := c.Param("id")

	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}

	h.appState.RLock()
	problem, ok := h.appState.Problems[problemID]
	if !ok {
		h.appState.RUnlock()
		util.Error(c, http.StatusNotFound, fmt.Errorf("problem not found"))
		return
	}

	parentContest, ok := h.appState.ProblemToContestMap[problemID]
	if !ok {
		h.appState.RUnlock()
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("internal server error: problem has no parent contest"))
		return
	}

	// Check if user is registered for the contest
	isRegistered, err := database.IsUserRegisteredForContest(h.db, user.ID, parentContest.ID)
	if err != nil {
		h.appState.RUnlock()
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to check contest registration: %w", err))
		return
	}
	if !isRegistered {
		h.appState.RUnlock()
		util.Error(c, http.StatusForbidden, fmt.Errorf("you must register for the contest before submitting"))
		return
	}

	// Check time restrictions for submission
	now := time.Now()
	if now.Before(parentContest.StartTime) || now.After(parentContest.EndTime) {
		h.appState.RUnlock()
		util.Error(c, http.StatusForbidden, fmt.Errorf("cannot submit because the contest is not active"))
		return
	}
	if now.Before(problem.StartTime) || now.After(problem.EndTime) {
		h.appState.RUnlock()
		util.Error(c, http.StatusForbidden, fmt.Errorf("cannot submit because the problem is not active"))
		return
	}
	h.appState.RUnlock()

	// Check submission limit
	if problem.MaxSubmissions > 0 {
		count, err := database.GetSubmissionCount(h.db, userID, parentContest.ID, problemID)
		if err != nil {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to check submission count: %w", err))
			return
		}
		if count >= problem.MaxSubmissions {
			util.Error(c, http.StatusForbidden, fmt.Errorf("maximum submission limit of %d reached", problem.MaxSubmissions))
			return
		}
	}

	form, err := c.MultipartForm()
	if err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	files := form.File["files"]

	submissionID := uuid.New().String()
	submissionPath := filepath.Join(h.cfg.Storage.SubmissionContent, submissionID)
	if err := os.MkdirAll(submissionPath, 0755); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	for _, file := range files {
		relativePath := filepath.Clean(file.Filename)

		if filepath.IsAbs(relativePath) || strings.HasPrefix(relativePath, "..") {
			util.Error(c, http.StatusBadRequest, fmt.Sprintf("invalid file path: %s", file.Filename))
			return
		}

		dst := filepath.Join(submissionPath, relativePath)

		dst = filepath.Clean(dst)

		if !strings.HasPrefix(dst, submissionPath) {
			util.Error(c, http.StatusBadRequest, fmt.Sprintf("invalid file path after join: %s", file.Filename))
			return
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to create directory: %w", err))
			return
		}

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

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := database.CreateSubmission(tx, &sub); err != nil {
			return err
		}
		return database.IncrementSubmissionCount(tx, user.ID, parentContest.ID, problemID)
	})

	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to create submission record: %w", err))
		return
	}

	h.scheduler.Submit(&sub, problem)
	util.Success(c, gin.H{"submission_id": submissionID}, "Submission received")
}

func (h *Handler) getProblemAttempts(c *gin.Context) {
	userID := c.GetString("userID")
	problemID := c.Param("id")

	h.appState.RLock()
	problem, ok := h.appState.Problems[problemID]
	if !ok {
		h.appState.RUnlock()
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}
	parentContest, ok := h.appState.ProblemToContestMap[problemID]
	if !ok {
		h.appState.RUnlock()
		util.Error(c, http.StatusInternalServerError, "internal server error: problem has no parent contest")
		return
	}
	h.appState.RUnlock()

	usedCount, err := database.GetSubmissionCount(h.db, userID, parentContest.ID, problemID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to retrieve submission count: %w", err))
		return
	}

	type AttemptsResponse struct {
		Limit     *int `json:"limit"`
		Used      int  `json:"used"`
		Remaining *int `json:"remaining"`
	}

	resp := AttemptsResponse{Used: usedCount}

	if problem.MaxSubmissions > 0 {
		limit := problem.MaxSubmissions
		remaining := limit - usedCount
		if remaining < 0 {
			remaining = 0
		}
		resp.Limit = &limit
		resp.Remaining = &remaining
	}

	util.Success(c, resp, "Submission attempts retrieved successfully")
}

func (h *Handler) getUserSubmissions(c *gin.Context) {
	userID := c.GetString("userID")
	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}
	subs, err := database.GetSubmissionsByUserID(h.db, user.ID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, subs, "ok")
}

func (h *Handler) getUserSubmission(c *gin.Context) {
	subID := c.Param("id")
	userID := c.GetString("userID")
	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}
	sub, err := database.GetSubmission(h.db, subID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}
	if sub.UserID != user.ID {
		util.Error(c, http.StatusForbidden, fmt.Errorf("you can only view your own submissions"))
		return
	}
	util.Success(c, sub, "ok")
}

func (h *Handler) interruptSubmission(c *gin.Context) {
	subID := c.Param("id")
	userID := c.GetString("userID")
	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}

	sub, err := database.GetSubmission(h.db, subID)
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
		if err := database.UpdateSubmission(h.db, sub); err != nil {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update submission status: %w", err))
			return
		}
		msg := pubsub.FormatMessage("error", "Submission interrupted by user.")
		pubsub.GetBroker().Publish(subID, msg)
		pubsub.GetBroker().CloseTopic(subID)
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

		msg := pubsub.FormatMessage("error", "Submission interrupted by user.")
		pubsub.GetBroker().Publish(subID, msg)
		pubsub.GetBroker().CloseTopic(subID)
		util.Success(c, nil, "Running submission interrupted successfully")

	case models.StatusSuccess, models.StatusFailed:
		util.Error(c, http.StatusBadRequest, "Submission has already finished and cannot be interrupted")

	default:
		util.Error(c, http.StatusInternalServerError, fmt.Sprintf("Unknown submission status: %s", sub.Status))
	}
}

func (h *Handler) getSubmissionQueuePosition(c *gin.Context) {
	subID := c.Param("id")
	userID := c.GetString("userID")

	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}

	sub, err := database.GetSubmission(h.db, subID)
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

	count, err := database.CountQueuedSubmissionsBefore(h.db, sub.Cluster, sub.CreatedAt)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	util.Success(c, gin.H{"position": count}, "Queue position retrieved successfully")
}

func (h *Handler) getContainerLog(c *gin.Context) {
	subID := c.Param("id")
	conID := c.Param("conID")
	userID := c.GetString("userID")

	_, err := database.GetUserByID(h.db, userID)
	if err != nil {
		util.Error(c, http.StatusNotFound, "user not found")
		return
	}

	sub, err := database.GetSubmission(h.db, subID)
	if err != nil {
		util.Error(c, http.StatusNotFound, "submission not found")
		return
	}

	// Authorization Check : Ownership
	if sub.UserID != userID {
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

	h.appState.RLock()
	problem, ok := h.appState.Problems[sub.ProblemID]
	h.appState.RUnlock()
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
}
