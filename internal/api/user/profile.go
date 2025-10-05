package user

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)

func (h *Handler) getUserProfile(c *gin.Context) {
	userID := c.GetString("userID")
	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}
	// Prepend API path to avatar filename if it's not a full URL
	if user.AvatarURL != "" && !strings.HasPrefix(user.AvatarURL, "http") {
		user.AvatarURL = fmt.Sprintf("/api/v1/assets/avatars/%s", user.AvatarURL)
	}
	util.Success(c, user, "ok")
}

func (h *Handler) updateUserProfile(c *gin.Context) {
	userID := c.GetString("userID")
	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}
	var reqBody struct {
		Nickname  string `json:"nickname"`
		Signature string `json:"signature"`
	}
	if err := c.ShouldBindJSON(&reqBody); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	user.Nickname = reqBody.Nickname
	user.Signature = reqBody.Signature
	if err := database.UpdateUser(h.db, user); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, user, "Profile updated")
}

func (h *Handler) uploadAvatar(c *gin.Context) {
	userID := c.GetString("userID")
	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}

	file, err := c.FormFile("avatar")
	if err != nil {
		util.Error(c, http.StatusBadRequest, "Avatar file not provided")
		return
	}

	ext := filepath.Ext(file.Filename)
	avatarFilename := fmt.Sprintf("%s%s", user.ID, ext)
	avatarPath := filepath.Join(h.cfg.Storage.UserAvatar, avatarFilename)

	if err := c.SaveUploadedFile(file, avatarPath); err != nil {
		util.Error(c, http.StatusInternalServerError, "Failed to save avatar")
		return
	}

	user.AvatarURL = avatarFilename // Store only the filename
	if err := database.UpdateUser(h.db, user); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, user, "Avatar updated")
}
