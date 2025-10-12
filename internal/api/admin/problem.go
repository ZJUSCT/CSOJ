package admin

import (
	"fmt"
	"net/http"

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

	h.appState.RLock()
	existingProblem, ok := h.appState.Problems[problemID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}

	// Preserve internal fields that are not part of the request body
	updatedProblem.BasePath = existingProblem.BasePath

	if err := judger.UpdateProblem(&updatedProblem); err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update problem files: %w", err))
		return
	}
	zap.S().Infof("admin updated problem '%s'", updatedProblem.ID)
	h.reload(c)
}

func (h *Handler) deleteProblem(c *gin.Context) {
	problemID := c.Param("id")

	h.appState.RLock()
	_, ok := h.appState.Problems[problemID]
	if !ok {
		h.appState.RUnlock()
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}
	contest, contestOk := h.appState.ProblemToContestMap[problemID]
	h.appState.RUnlock()
	if !contestOk {
		util.Error(c, http.StatusInternalServerError, "could not find parent contest for problem, state may be inconsistent")
		return
	}

	if err := judger.DeleteProblem(contest, problemID); err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to delete problem files: %w", err))
		return
	}
	zap.S().Warnf("admin deleted problem '%s' from contest '%s'", problemID, contest.ID)
	h.reload(c)
}
