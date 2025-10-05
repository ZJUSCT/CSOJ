package user

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)

func (h *Handler) getProblem(c *gin.Context) {
	problemID := c.Param("id")
	h.appState.RLock()
	problem, ok := h.appState.Problems[problemID]
	if ok {
		parentContest, parentOk := h.appState.ProblemToContestMap[problemID]
		ok = parentOk
		if ok {
			now := time.Now()
			// Check if the contest and problem are active
			if now.Before(parentContest.StartTime) {
				util.Error(c, http.StatusForbidden, fmt.Errorf("contest has not started yet"))
				h.appState.RUnlock()
				return
			}
			if now.Before(problem.StartTime) {
				util.Error(c, http.StatusForbidden, fmt.Errorf("problem has not started yet"))
				h.appState.RUnlock()
				return
			}
		} else {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("internal server error: problem has no parent contest"))
			h.appState.RUnlock()
			return
		}
	}
	h.appState.RUnlock()

	if !ok {
		util.Error(c, http.StatusNotFound, fmt.Errorf("problem not found"))
		return
	}

	util.Success(c, problem, "Problem found")
}
