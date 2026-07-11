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

// getAllProblems returns a list of all loaded problems.
func (h *Handler) getAllProblems(c *gin.Context) {
	h.appState.RLock()
	defer h.appState.RUnlock()
	util.Success(c, h.appState.Problems, "All loaded problems retrieved")
}

// getProblem returns the full definition of a single problem, with no time restrictions.
func (h *Handler) getProblem(c *gin.Context) {
	problemID := c.Param("id")

	h.appState.RLock()
	problem, ok := h.appState.Problems[problemID]
	h.appState.RUnlock()

	if !ok {
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}

	// Unlike the user API, there are no authorization checks based on contest/problem times.
	// We also return the full problem struct, not a stripped-down response model.
	util.Success(c, problem, "Problem definition retrieved")
}

func (h *Handler) updateProblem(c *gin.Context) {
	problemID := c.Param("id")
	var updatedProblem judger.Problem
	if err := c.ShouldBindJSON(&updatedProblem); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if problemID != updatedProblem.ID {
		util.Error(c, http.StatusBadRequest, "problem ID in path does not match problem ID in body")
		return
	}
	if err := judger.NormalizeProblemGPUResources(&updatedProblem); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	h.appState.RLock()
	existing, ok := h.appState.Problems[problemID]
	parentContest, _ := h.appState.ProblemToContestMap[problemID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}

	// Preserve the cluster assignment (it's part of the body normally, but be defensive).
	_ = existing

	contestID := ""
	if parentContest != nil {
		contestID = parentContest.ID
	}
	mp, err := judger.ProblemToModel(&updatedProblem, contestID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to marshal problem: %w", err))
		return
	}
	if err := h.db.Save(&mp).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update problem: %w", err))
		return
	}
	zap.S().Infof("admin updated problem '%s'", updatedProblem.ID)
	h.reload(c)
}

func (h *Handler) deleteProblem(c *gin.Context) {
	problemID := c.Param("id")

	h.appState.RLock()
	_, ok := h.appState.Problems[problemID]
	parentContest, contestOk := h.appState.ProblemToContestMap[problemID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}
	if !contestOk || parentContest == nil {
		util.Error(c, http.StatusInternalServerError, "could not find parent contest for problem, state may be inconsistent")
		return
	}

	if err := h.db.Delete(&models.Problem{}, "id = ?", problemID).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to delete problem: %w", err))
		return
	}

	// Remove the problem ID from the parent contest's ordered list
	newIDs := make([]string, 0, len(parentContest.ProblemIDs))
	for _, pid := range parentContest.ProblemIDs {
		if pid != problemID {
			newIDs = append(newIDs, pid)
		}
	}
	if err := h.db.Model(&models.Contest{}).Where("id = ?", parentContest.ID).Update("problem_ids", models.StringArray(newIDs)).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update contest problem list: %w", err))
		return
	}
	zap.S().Warnf("admin deleted problem '%s' from contest '%s'", problemID, parentContest.ID)
	h.reload(c)
}
