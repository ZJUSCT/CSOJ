package admin

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/judger"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// getAllContests returns a list of all loaded contests, regardless of their start/end times.
func (h *Handler) getAllContests(c *gin.Context) {
	h.appState.RLock()
	defer h.appState.RUnlock()

	// Unlike the user API, the admin API returns all contests with all details at all times.
	util.Success(c, h.appState.Contests, "All loaded contests retrieved")
}

// getContest returns details for a specific contest, regardless of its start/end time.
func (h *Handler) getContest(c *gin.Context) {
	contestID := c.Param("id")
	h.appState.RLock()
	contest, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()

	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}
	// Unlike the user API, the admin API returns full contest details at all times.
	util.Success(c, contest, "Contest details retrieved")
}

func (h *Handler) createContest(c *gin.Context) {
	var newContest judger.Contest
	if err := c.ShouldBindJSON(&newContest); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	h.appState.RLock()
	_, exists := h.appState.Contests[newContest.ID]
	h.appState.RUnlock()
	if exists {
		util.Error(c, http.StatusConflict, "a contest with this ID already exists")
		return
	}

	mc := judger.ContestToModel(&newContest)
	if err := h.db.Create(&mc).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to create contest: %w", err))
		return
	}
	zap.S().Infof("admin created contest '%s'", newContest.ID)
	h.reload(c)
}

func (h *Handler) updateContest(c *gin.Context) {
	contestID := c.Param("id")
	var updatedContest judger.Contest
	if err := c.ShouldBindJSON(&updatedContest); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if contestID != updatedContest.ID {
		util.Error(c, http.StatusBadRequest, "contest ID in path does not match contest ID in body")
		return
	}

	h.appState.RLock()
	existing, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	// Preserve the problem list (managed via problem endpoints / order endpoint)
	updatedContest.ProblemIDs = existing.ProblemIDs
	mc := judger.ContestToModel(&updatedContest)
	if err := h.db.Save(&mc).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update contest: %w", err))
		return
	}
	zap.S().Infof("admin updated contest '%s'", updatedContest.ID)
	h.reload(c)
}

// handleUpdateContestProblemOrder updates the order of problems in a contest.
func (h *Handler) handleUpdateContestProblemOrder(c *gin.Context) {
	contestID := c.Param("id")
	var req struct {
		ProblemIDs []string `json:"problem_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	h.appState.RLock()
	contest, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	newSet := make(map[string]struct{})
	for _, pid := range req.ProblemIDs {
		if _, exists := newSet[pid]; exists {
			util.Error(c, http.StatusBadRequest, fmt.Sprintf("duplicate problem ID in request: %s", pid))
			return
		}
		newSet[pid] = struct{}{}
	}
	origSet := make(map[string]struct{})
	for _, pid := range contest.ProblemIDs {
		origSet[pid] = struct{}{}
	}
	if len(newSet) != len(origSet) {
		util.Error(c, http.StatusBadRequest, "number of problems does not match original")
		return
	}
	for pid := range newSet {
		if _, exists := origSet[pid]; !exists {
			util.Error(c, http.StatusBadRequest, fmt.Sprintf("problem ID %s not found in original contest", pid))
			return
		}
	}

	newIDs := models.StringArray(req.ProblemIDs)
	if err := h.db.Model(&models.Contest{}).Where("id = ?", contestID).Update("problem_ids", newIDs).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update contest problem order: %w", err))
		return
	}
	zap.S().Infof("admin updated problem order for contest '%s'", contestID)
	h.reload(c)
}

func (h *Handler) deleteContest(c *gin.Context) {
	contestID := c.Param("id")
	if err := h.db.Delete(&models.Contest{}, "id = ?", contestID).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to delete contest: %w", err))
		return
	}
	zap.S().Warnf("admin deleted contest '%s'", contestID)
	h.reload(c)
}

func (h *Handler) createProblemInContest(c *gin.Context) {
	contestID := c.Param("id")
	var newProblem judger.Problem
	if err := c.ShouldBindJSON(&newProblem); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	h.appState.RLock()
	contest, ok := h.appState.Contests[contestID]
	if !ok {
		h.appState.RUnlock()
		util.Error(c, http.StatusNotFound, "parent contest not found")
		return
	}
	_, problemExists := h.appState.Problems[newProblem.ID]
	h.appState.RUnlock()
	if problemExists {
		util.Error(c, http.StatusConflict, "a problem with this ID already exists")
		return
	}

	mp, err := judger.ProblemToModel(&newProblem, contestID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to marshal problem: %w", err))
		return
	}
	if err := h.db.Create(&mp).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to create problem: %w", err))
		return
	}

	// Append the problem ID to the contest's ordered list
	newIDs := append(contest.ProblemIDs, newProblem.ID)
	if err := h.db.Model(&models.Contest{}).Where("id = ?", contestID).Update("problem_ids", models.StringArray(newIDs)).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update contest problem list: %w", err))
		return
	}
	zap.S().Infof("admin created problem '%s' in contest '%s'", newProblem.ID, contestID)
	h.reload(c)
}

// getContestLeaderboard provides an admin-accessible endpoint for the contest leaderboard.
func (h *Handler) getContestLeaderboard(c *gin.Context) {
	contestID := c.Param("id")
	h.appState.RLock()
	// Add tag query parameter
	tags := c.Query("tags") // Comma-separated string of tags
	_, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}
	leaderboard, err := database.GetLeaderboard(h.db, contestID, tags)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, leaderboard, "Leaderboard retrieved")
}

// getContestTrend provides an admin-accessible endpoint for the contest score trend.
func (h *Handler) getContestTrend(c *gin.Context) {
	contestID := c.Param("id")
	maxnum := c.DefaultQuery("maxnum", "20")

	maxnumInt, err := strconv.Atoi(maxnum)
	if err != nil || maxnumInt <= 0 {
		util.Error(c, http.StatusBadRequest, "invalid maxnum parameter")
		return
	}

	h.appState.RLock()
	_, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}
	// This logic is copied from user/contest.go and is fine for admin use.
	leaderboard, err := database.GetLeaderboard(h.db, contestID, "") // Trend doesn't support tag filtering for now
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	var topUsers []database.LeaderboardEntry
	topUserIDs := make([]string, 0)
	tenthScore := -1

	for _, entry := range leaderboard {
		if entry.TotalScore == 0 {
			continue
		}
		if len(topUsers) < maxnumInt {
			topUsers = append(topUsers, entry)
			topUserIDs = append(topUserIDs, entry.UserID)
			if len(topUsers) == maxnumInt {
				tenthScore = entry.TotalScore
			}
		} else if tenthScore != -1 && entry.TotalScore == tenthScore {
			topUsers = append(topUsers, entry)
			topUserIDs = append(topUserIDs, entry.UserID)
		}
	}

	if len(topUserIDs) == 0 {
		util.Success(c, make([]interface{}, 0), "Trend data retrieved")
		return
	}

	histories, err := database.GetScoreHistoriesForUsers(h.db, contestID, topUserIDs)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	type TrendEntry struct {
		UserID   string                           `json:"user_id"`
		Username string                           `json:"username"`
		Nickname string                           `json:"nickname"`
		History  []database.UserScoreHistoryPoint `json:"history"`
	}

	trendData := make([]TrendEntry, 0, len(topUsers))
	for _, user := range topUsers {
		userHistory, ok := histories[user.UserID]
		if !ok {
			userHistory = []database.UserScoreHistoryPoint{}
		}

		trendData = append(trendData, TrendEntry{
			UserID:   user.UserID,
			Username: user.Username,
			Nickname: user.Nickname,
			History:  userHistory,
		})
	}

	util.Success(c, trendData, "Trend data retrieved")
}
