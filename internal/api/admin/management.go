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
	// Load new data into temporary variables
	zap.S().Info("starting reload process...")

	// Find contest directories from the root
	contestDirs, err := judger.FindContestDirs(h.cfg.ContestsRoot)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to scan contests_root directory: %w", err))
		return
	}
	zap.S().Infof("found %d contest directories in '%s'", len(contestDirs), h.cfg.ContestsRoot)

	// Load all contests and problems from the found directories
	newContests, newProblems, err := judger.LoadAllContestsAndProblems(contestDirs)
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
	if err := h.db.Preload("Containers").Find(&allSubmissions).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to get all submissions: %w", err))
		return
	}

	// Create new Problem-to-Contest map
	newProblemToContestMap := make(map[string]*judger.Contest)
	for _, contest := range newContests {
		for _, problemID := range contest.ProblemIDs {
			newProblemToContestMap[problemID] = contest
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
