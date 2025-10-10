package user

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)

func (h *Handler) getLinks(c *gin.Context) {
	if h.cfg.Links == nil {
		// Ensure we return an empty array instead of null if links are not configured
		util.Success(c, []interface{}{}, "Links retrieved successfully")
		return
	}
	util.Success(c, h.cfg.Links, "Links retrieved successfully")
}

func (h *Handler) getAllContests(c *gin.Context) {
	h.appState.RLock()
	defer h.appState.RUnlock()
	util.Success(c, h.appState.Contests, "Contests loaded")
}

func (h *Handler) getContest(c *gin.Context) {
	contestID := c.Param("id")
	h.appState.RLock()
	contest, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()

	if !ok {
		util.Error(c, http.StatusNotFound, fmt.Errorf("contest not found"))
		return
	}

	now := time.Now()
	// For contests that haven't started or have already ended, hide the problem list.
	if now.Before(contest.StartTime) || now.After(contest.EndTime) {
		// Create a copy to avoid modifying the original map entry
		contestCopy := *contest
		contestCopy.ProblemIDs = []string{} // Empty the problem list
		util.Success(c, contestCopy, "Contest found, but is not currently active")
		return
	}
	util.Success(c, contest, "Contest found")
}

func (h *Handler) getContestLeaderboard(c *gin.Context) {
	contestID := c.Param("id")
	leaderboard, err := database.GetLeaderboard(h.db, contestID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, leaderboard, "Leaderboard retrieved")
}

func (h *Handler) getContestTrend(c *gin.Context) {
	contestID := c.Param("id")
	leaderboard, err := database.GetLeaderboard(h.db, contestID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	// Determine top 10 users (with ties, score > 0)
	var topUsers []database.LeaderboardEntry
	topUserIDs := make([]string, 0)
	tenthScore := -1

	for _, entry := range leaderboard {
		if entry.TotalScore == 0 {
			continue
		}

		if len(topUsers) < 10 {
			topUsers = append(topUsers, entry)
			topUserIDs = append(topUserIDs, entry.UserID)
			if len(topUsers) == 10 {
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

	// Get score histories for these users
	histories, err := database.GetScoreHistoriesForUsers(h.db, contestID, topUserIDs)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	// Response structure
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

func (h *Handler) registerForContest(c *gin.Context) {
	userID := c.GetString("userID")
	contestID := c.Param("id")

	h.appState.RLock()
	contest, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()

	if !ok {
		util.Error(c, http.StatusNotFound, fmt.Errorf("contest not found"))
		return
	}

	now := time.Now()
	if now.Before(contest.StartTime) {
		util.Error(c, http.StatusForbidden, fmt.Errorf("contest has not started, cannot register"))
		return
	}
	if now.After(contest.EndTime) {
		util.Error(c, http.StatusForbidden, fmt.Errorf("contest has ended, cannot register"))
		return
	}

	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}

	if err := database.RegisterForContest(h.db, user.ID, contestID); err != nil {
		if err.Error() == "already registered" {
			util.Error(c, http.StatusConflict, err)
			return
		}
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, nil, "Successfully registered for contest")
}

func (h *Handler) getContestHistory(c *gin.Context) {
	userID := c.GetString("userID")
	contestID := c.Param("id")

	h.appState.RLock()
	_, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()

	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	history, err := database.GetScoreHistoryForUser(h.db, contestID, userID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	util.Success(c, history, "User score history retrieved successfully")
}
