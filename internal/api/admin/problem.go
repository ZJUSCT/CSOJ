package admin

import (
	"net/http"

	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
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
