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

	// Compute effective end time. A matching tag override REPLACES the default
	// deadline (even if earlier); the latest matching override wins. Anonymous
	// callers (no userID from OptionalAuthMiddleware) keep the default window.
	var defaultEnd time.Time
	if problem.SubmitEndTime != nil {
		defaultEnd = *problem.SubmitEndTime
	} else {
		defaultEnd = problem.EndTime
	}
	effectiveEnd := defaultEnd
	if userID != "" {
		if u, err := database.GetUserByID(h.db, userID); err == nil && u != nil {
			if t, ok := latestMatchingOverride(problem, u.Tags, defaultEnd); ok {
				effectiveEnd = t
			}
		}
	}
	response.EffectiveEndTime = &effectiveEnd

	util.Success(c, response, "Problem found")
}

// latestMatchingOverride implements "match replaces default" semantics for the
// tag-based deadline overrides: if the user's tags match any override rule,
// the default deadline is replaced — even if the matched override is earlier.
// When multiple overrides match, the latest (most favorable) end time wins.
// It always returns a valid deadline: defaultEnd when no rule matches (the ok
// value is false then), the matched override time otherwise (ok true).
func latestMatchingOverride(problem *judger.Problem, userTagsCSV string, defaultEnd time.Time) (time.Time, bool) {
	if len(problem.DeadlineOverrides) == 0 {
		return defaultEnd, false
	}
	userTags := strings.Split(userTagsCSV, ",")
	matched := false
	var latest time.Time
	for _, override := range problem.DeadlineOverrides {
		hit := false
		for _, userTag := range userTags {
			for _, overrideTag := range override.Tags {
				if strings.TrimSpace(userTag) != "" && strings.TrimSpace(userTag) == strings.TrimSpace(overrideTag) {
					hit = true
					break
				}
			}
			if hit {
				break
			}
		}
		if !hit {
			continue
		}
		overrideTime, err := time.Parse(time.RFC3339, override.EndTime)
		if err != nil {
			continue // invalid override date: skip, don't apply
		}
		if !matched || overrideTime.After(latest) {
			latest = overrideTime
			matched = true
		}
	}
	if !matched {
		return defaultEnd, false
	}
	return latest, true
}
