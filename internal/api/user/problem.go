package user

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/judger"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)

type WorkflowStepResponse struct {
	Name string `json:"name"`
	Show bool   `json:"show"`
}

type ProblemResponse struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	StartTime      time.Time              `json:"starttime"`
	EndTime        time.Time              `json:"endtime"`
	MaxSubmissions int                    `json:"max_submissions"`
	Cluster        string                 `json:"cluster"`
	CPU            int                    `json:"cpu"`
	Memory         int64                  `json:"memory"`
	Upload         judger.UploadLimit     `json:"upload"`
	Workflow       []WorkflowStepResponse `json:"workflow"`
	Description    string                 `json:"description"`
}

func (h *Handler) getProblem(c *gin.Context) {
	problemID := c.Param("id")
	h.appState.RLock()
	problem, ok := h.appState.Problems[problemID]
	if ok {
		parentContest, parentOk := h.appState.ProblemToContestMap[problemID]
		ok = parentOk
		if ok {
			now := time.Now()
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

	workflowResponse := make([]WorkflowStepResponse, len(problem.Workflow))
	for i, step := range problem.Workflow {
		workflowResponse[i] = WorkflowStepResponse{Name: step.Name, Show: step.Show}
	}

	response := ProblemResponse{
		ID:             problem.ID,
		Name:           problem.Name,
		StartTime:      problem.StartTime,
		EndTime:        problem.EndTime,
		MaxSubmissions: problem.MaxSubmissions,
		Cluster:        problem.Cluster,
		CPU:            problem.CPU,
		Memory:         problem.Memory,
		Upload:         problem.Upload,
		Workflow:       workflowResponse,
		Description:    problem.Description,
	}

	util.Success(c, response, "Problem found")
}
