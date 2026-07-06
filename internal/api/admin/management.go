package admin

import (
	"fmt"
	"net/http"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/judger"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (h *Handler) reload(c *gin.Context) {
	zap.S().Info("starting reload process...")

	newContests, newProblems, newProblemToContestMap, err := judger.LoadFromDB(h.db)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to load contests/problems from DB: %w", err))
		return
	}
	zap.S().Infof("loaded %d contests and %d problems from DB", len(newContests), len(newProblems))

	newProblemIDs := make(map[string]struct{}, len(newProblems))
	for id := range newProblems {
		newProblemIDs[id] = struct{}{}
	}

	// Find submissions whose problems have been deleted
	var allSubmissions []models.Submission
	if err := h.db.Preload("Containers").Find(&allSubmissions).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to get all submissions: %w", err))
		return
	}

	for _, sub := range allSubmissions {
		if _, ok := newProblemIDs[sub.ProblemID]; ok {
			continue
		}
		// Problem was deleted, remove the submission and its containers
		zap.S().Infof("problem '%s' no longer exists, deleting submission %s and its containers", sub.ProblemID, sub.ID)
		for _, container := range sub.Containers {
			if err := h.db.Unscoped().Delete(&container).Error; err != nil {
				zap.S().Errorf("failed to hard-delete container %s: %v", container.ID, err)
			}
		}
		if err := h.db.Unscoped().Delete(&sub).Error; err != nil {
			zap.S().Errorf("failed to hard-delete submission %s: %v", sub.ID, err)
		}
	}

	// Atomically update the shared state
	h.appState.Lock()
	h.appState.Contests = newContests
	h.appState.Problems = newProblems
	h.appState.ProblemToContestMap = newProblemToContestMap
	h.appState.Unlock()
	zap.S().Info("app state reloaded successfully")

	util.Success(c, gin.H{
		"contests_loaded": len(newContests),
		"problems_loaded": len(newProblems),
	}, "Reload successful")
}
