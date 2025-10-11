package admin

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"math"
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
	// Pagination parameters
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "20")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 20
	}
	if limit > 100 { // Add a reasonable upper bound for limit
		limit = 100
	}

	offset := (page - 1) * limit

	// Base query for filtering
	query := h.db.Model(&models.Submission{})

	if problemID := c.Query("problem_id"); problemID != "" {
		query = query.Where("submissions.problem_id = ?", problemID)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("submissions.status = ?", status)
	}
	if userQuery := c.Query("user_query"); userQuery != "" {
		likeQuery := "%" + userQuery + "%"
		// Join with users table to filter by user attributes
		query = query.Joins("JOIN users ON users.id = submissions.user_id").
			Where("users.id = ? OR users.username LIKE ? OR users.nickname LIKE ?", userQuery, likeQuery, likeQuery)
	}

	// Get total count
	var totalItems int64
	if err := query.Count(&totalItems).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	// Get paginated results
	var subs []models.Submission
	// We need to apply the same joins for the final query as for the count query
	// but the `query` variable already has them. We just need to add the preload and specify the table for ordering.
	if err := query.Preload("User").Order("submissions.created_at DESC").Offset(offset).Limit(limit).Find(&subs).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	totalPages := int(math.Ceil(float64(totalItems) / float64(limit)))

	response := gin.H{
		"items":        subs,
		"total_items":  totalItems,
		"total_pages":  totalPages,
		"current_page": page,
		"per_page":     limit,
	}

	util.Success(c, response, "Submissions retrieved successfully")
}

func (h *Handler) getSubmission(c *gin.Context) {
	sub, err := database.GetSubmission(h.db, c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}
	util.Success(c, sub, "ok")
}

func (h *Handler) getSubmissionContent(c *gin.Context) {
	subID := c.Param("id")

	// Check if submission exists
	_, err := database.GetSubmission(h.db, subID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			util.Error(c, http.StatusNotFound, "submission not found")
		} else {
			util.Error(c, http.StatusInternalServerError, err)
		}
		return
	}

	submissionPath := filepath.Join(h.cfg.Storage.SubmissionContent, subID)

	// Check if the directory exists
	info, err := os.Stat(submissionPath)
	if os.IsNotExist(err) || !info.IsDir() {
		util.Error(c, http.StatusNotFound, "submission content not found on disk")
		return
	}

	// Create a buffer to write our archive to.
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	// Walk the directory and add files to the zip.
	err = filepath.Walk(submissionPath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create a proper zip header
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		// Update the header name to be relative to the submission directory
		relPath, err := filepath.Rel(submissionPath, path)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relPath) // Use forward slashes in zip

		// If it's a directory, just create the header
		if info.IsDir() {
			header.Name += "/"
		} else {
			// Set compression method
			header.Method = zip.Deflate
		}

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		// If it's a file, write its content to the zip
		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(writer, file)
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		zap.S().Errorf("failed to create zip archive for submission %s: %v", subID, err)
		util.Error(c, http.StatusInternalServerError, "failed to create zip archive")
		return
	}

	// Close the zip writer to finalize the archive
	zipWriter.Close()

	// Set headers for file download
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"submission_%s.zip\"", subID))
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

func (h *Handler) updateSubmission(c *gin.Context) {
	subID := c.Param("id")
	sub, err := database.GetSubmission(h.db, subID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}

	var req struct {
		Status *models.Status  `json:"status"`
		Score  *int            `json:"score"`
		Info   *models.JSONMap `json:"info"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	if req.Status != nil {
		sub.Status = *req.Status
	}
	if req.Score != nil {
		sub.Score = *req.Score
	}
	if req.Info != nil {
		sub.Info = *req.Info
	}

	if err := database.UpdateSubmission(h.db, sub); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	zap.S().Warnf("admin manually updated submission %s", sub.ID)
	util.Success(c, sub, "Submission manually updated. Note: This does not trigger score recalculation.")
}

func (h *Handler) deleteSubmission(c *gin.Context) {
	subID := c.Param("id")
	// First, get submission to find its content path, if any.
	sub, err := database.GetSubmission(h.db, subID)
	if err != nil {
		util.Error(c, http.StatusNotFound, "submission not found")
		return
	}

	// Delete from DB. GORM's cascading delete will handle associated containers.
	if err := h.db.Delete(&models.Submission{}, subID).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to delete submission from database: %w", err))
		return
	}

	// Delete submission content from disk.
	submissionPath := filepath.Join(h.cfg.Storage.SubmissionContent, subID)
	if err := os.RemoveAll(submissionPath); err != nil {
		zap.S().Errorf("failed to delete submission content at %s: %v", submissionPath, err)
		util.Error(c, http.StatusInternalServerError, "DB record deleted, but failed to delete submission content from disk")
		return
	}
	zap.S().Warnf("admin deleted submission %s and its content", sub.ID)
	util.Success(c, nil, "Submission and its content deleted successfully")
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
		problem, probOk := h.appState.Problems[sub.ProblemID]
		h.appState.RUnlock()
		if !ok || !probOk {
			// This should not happen in a consistent system, but handle it
			zap.S().Errorf("failed to find parent contest or problem %s during score recalculation for submission %s", sub.ProblemID, sub.ID)
		} else {
			if err := database.RecalculateScoresForUserProblem(h.db, sub.UserID, sub.ProblemID, contest.ID, sub.ID, problem.Score.Mode, problem.Score.MaxPerformanceScore); err != nil {
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
