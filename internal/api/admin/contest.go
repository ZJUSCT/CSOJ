package admin

import (
	"net/http"
	"strconv"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
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

// getContestLeaderboard provides an admin-accessible endpoint for the contest leaderboard.
func (h *Handler) getContestLeaderboard(c *gin.Context) {
	contestID := c.Param("id")
	h.appState.RLock()
	_, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	leaderboard, err := database.GetLeaderboard(h.db, contestID)
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
	leaderboard, err := database.GetLeaderboard(h.db, contestID)
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
