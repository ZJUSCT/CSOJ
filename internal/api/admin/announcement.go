package admin

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func (h *Handler) handleGetContestAnnouncements(c *gin.Context) {
	contestID := c.Param("id")
	h.appState.RLock()
	contest, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}
	util.Success(c, contest.Announcements, "Announcements retrieved successfully")
}

func (h *Handler) handleCreateContestAnnouncement(c *gin.Context) {
	contestID := c.Param("id")
	var req struct {
		Title       string `json:"title" binding:"required"`
		Description string `json:"description" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	h.appState.RLock()
	_, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	now := time.Now()
	ann := models.Announcement{
		ID:          uuid.NewString(),
		ContestID:   contestID,
		Title:       req.Title,
		Description: req.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := h.db.Create(&ann).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to create announcement: %w", err))
		return
	}
	zap.S().Infof("admin created announcement '%s' in contest '%s'", ann.ID, contestID)
	h.reload(c)
}

func (h *Handler) handleUpdateContestAnnouncement(c *gin.Context) {
	contestID := c.Param("id")
	announcementID := c.Param("announcementId")
	var req struct {
		Title       string `json:"title" binding:"required"`
		Description string `json:"description" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	h.appState.RLock()
	_, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	res := h.db.Model(&models.Announcement{}).Where("id = ? AND contest_id = ?", announcementID, contestID).
		Updates(map[string]interface{}{"title": req.Title, "description": req.Description, "updated_at": time.Now()})
	if res.Error != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update announcement: %w", res.Error))
		return
	}
	if res.RowsAffected == 0 {
		util.Error(c, http.StatusNotFound, "announcement not found")
		return
	}
	zap.S().Infof("admin updated announcement '%s' in contest '%s'", announcementID, contestID)
	h.reload(c)
}

func (h *Handler) handleDeleteContestAnnouncement(c *gin.Context) {
	contestID := c.Param("id")
	announcementID := c.Param("announcementId")

	res := h.db.Where("id = ? AND contest_id = ?", announcementID, contestID).Delete(&models.Announcement{})
	if res.Error != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to delete announcement: %w", res.Error))
		return
	}
	if res.RowsAffected == 0 {
		util.Error(c, http.StatusNotFound, "announcement not found")
		return
	}
	zap.S().Warnf("admin deleted announcement '%s' from contest '%s'", announcementID, contestID)
	h.reload(c)
}
