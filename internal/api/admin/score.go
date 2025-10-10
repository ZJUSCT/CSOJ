package admin

import (
	"fmt"
	"net/http"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (h *Handler) recalculateScore(c *gin.Context) {
	var req struct {
		UserID    string `json:"user_id" binding:"required"`
		ProblemID string `json:"problem_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	h.appState.RLock()
	problem, ok := h.appState.Problems[req.ProblemID]
	if !ok {
		h.appState.RUnlock()
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}
	contest, contestOk := h.appState.ProblemToContestMap[req.ProblemID]
	if !contestOk {
		h.appState.RUnlock()
		util.Error(c, http.StatusInternalServerError, "could not find parent contest for problem")
		return
	}
	h.appState.RUnlock()

	// Using an empty submission ID for the source, as this is an admin-triggered action.
	err := database.RecalculateScoresForUserProblem(h.db, req.UserID, req.ProblemID, contest.ID, "admin-recalc", problem.Score.Mode, problem.Score.MaxPerformanceScore)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to recalculate scores: %w", err))
		return
	}

	zap.S().Infof("admin triggered score recalculation for user %s on problem %s", req.UserID, req.ProblemID)
	util.Success(c, nil, "Score recalculation triggered successfully")
}
