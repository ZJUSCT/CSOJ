package admin

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/judger"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// Helper to read announcements file
func readAnnouncementsFile(path string) ([]*judger.Announcement, error) {
	var announcements []*judger.Announcement
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return announcements, nil // Return empty slice if file doesn't exist
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(data, &announcements)
	return announcements, err
}

// Helper to write announcements file
func writeAnnouncementsFile(path string, announcements []*judger.Announcement) error {
	data, err := yaml.Marshal(announcements)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// handleGetContestAnnouncements retrieves all announcements for a specific contest.
func (h *Handler) handleGetContestAnnouncements(c *gin.Context) {
	contestID := c.Param("id")
	h.appState.RLock()
	contest, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}
	// The announcements are already loaded in memory, return them directly.
	util.Success(c, contest.Announcements, "Announcements retrieved successfully")
}

// handleCreateContestAnnouncement creates a new announcement for a contest.
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
	contest, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	announcementsPath := filepath.Join(contest.BasePath, "announcements.yaml")
	announcements, err := readAnnouncementsFile(announcementsPath)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to read announcements file: %w", err))
		return
	}

	newAnn := &judger.Announcement{
		ID:          uuid.NewString(),
		Title:       req.Title,
		Description: req.Description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	announcements = append(announcements, newAnn)

	if err := writeAnnouncementsFile(announcementsPath, announcements); err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to write announcements file: %w", err))
		return
	}
	zap.S().Infof("admin created announcement '%s' in contest '%s'", newAnn.ID, contestID)
	h.reload(c)
}

// handleUpdateContestAnnouncement updates an existing announcement.
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
	contest, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	announcementsPath := filepath.Join(contest.BasePath, "announcements.yaml")
	announcements, err := readAnnouncementsFile(announcementsPath)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to read announcements file: %w", err))
		return
	}

	found := false
	for _, ann := range announcements {
		if ann.ID == announcementID {
			ann.Title = req.Title
			ann.Description = req.Description
			ann.UpdatedAt = time.Now()
			found = true
			break
		}
	}

	if !found {
		util.Error(c, http.StatusNotFound, "announcement not found")
		return
	}

	if err := writeAnnouncementsFile(announcementsPath, announcements); err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to write announcements file: %w", err))
		return
	}
	zap.S().Infof("admin updated announcement '%s' in contest '%s'", announcementID, contestID)
	h.reload(c)
}

// handleDeleteContestAnnouncement deletes an announcement.
func (h *Handler) handleDeleteContestAnnouncement(c *gin.Context) {
	contestID := c.Param("id")
	announcementID := c.Param("announcementId")

	h.appState.RLock()
	contest, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	announcementsPath := filepath.Join(contest.BasePath, "announcements.yaml")
	announcements, err := readAnnouncementsFile(announcementsPath)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to read announcements file: %w", err))
		return
	}

	var newAnnouncements []*judger.Announcement
	found := false
	for _, ann := range announcements {
		if ann.ID == announcementID {
			found = true
			continue
		}
		newAnnouncements = append(newAnnouncements, ann)
	}

	if !found {
		util.Error(c, http.StatusNotFound, "announcement not found")
		return
	}

	if err := writeAnnouncementsFile(announcementsPath, newAnnouncements); err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to write announcements file: %w", err))
		return
	}
	zap.S().Warnf("admin deleted announcement '%s' from contest '%s'", announcementID, contestID)
	h.reload(c)
}
