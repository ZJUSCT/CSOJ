package user

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/judger"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)

type WorkflowStepResponse struct {
	Name string `json:"name"`
	Show bool   `json:"show"`
}

type ProblemResponse struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Level            string                 `yaml:"level" json:"level"`
	StartTime        time.Time              `json:"starttime"`
	EndTime          time.Time              `json:"endtime"`
	SubmitStartTime  *time.Time             `json:"submit_start_time,omitempty"`
	SubmitEndTime    *time.Time             `json:"submit_end_time,omitempty"`
	EffectiveEndTime *time.Time             `json:"effective_end_time,omitempty"`
	MaxSubmissions   int                    `json:"max_submissions"`
	Cluster          string                 `json:"cluster"`
	Upload           judger.UploadLimit     `json:"upload"`
	Workflow         []WorkflowStepResponse `json:"workflow"`
	Score            judger.ScoreConfig     `json:"score"`
	Description      string                 `json:"description"`
}

func (h *Handler) getProblem(c *gin.Context) {
	problemID := c.Param("id")
	userID := c.GetString("userID")
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
		ID:              problem.ID,
		Name:            problem.Name,
		Level:           problem.Level,
		StartTime:       problem.StartTime,
		EndTime:         problem.EndTime,
		SubmitStartTime: problem.SubmitStartTime,
		SubmitEndTime:   problem.SubmitEndTime,
		MaxSubmissions:  problem.MaxSubmissions,
		Cluster:         problem.Cluster,
		Upload:          problem.Upload,
		Workflow:        workflowResponse,
		Score:  	    problem.Score,
		Description:     problem.Description,
	}

	// Compute effective end time based on tag overrides
	var effectiveEnd *time.Time
	if problem.SubmitEndTime != nil {
		effectiveEnd = problem.SubmitEndTime
	} else {
		endTime := problem.EndTime
		effectiveEnd = &endTime
	}

	if len(problem.DeadlineOverrides) > 0 {
		user, _ := database.GetUserByID(h.db, userID)
		userTags := strings.Split(user.Tags, ",")
		for _, override := range problem.DeadlineOverrides {
			matched := false
			for _, userTag := range userTags {
				for _, overrideTag := range override.Tags {
					if strings.TrimSpace(userTag) != "" && strings.TrimSpace(userTag) == strings.TrimSpace(overrideTag) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if matched {
				overrideTime, err := time.Parse(time.RFC3339, override.EndTime)
				if err == nil && overrideTime.After(*effectiveEnd) {
					effectiveEnd = &overrideTime
				}
			}
		}
	}
	response.EffectiveEndTime = effectiveEnd

	util.Success(c, response, "Problem found")
}
