package user

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
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

func validateAvatar(file *multipart.FileHeader) error {
	const maxAvatarSize = 5 * 1024 * 1024
	if file.Size > maxAvatarSize {
		return fmt.Errorf("avatar file is too large. Maximum size is 5MB")
	}

	src, err := file.Open()
	if err != nil {
		return fmt.Errorf("could not open file for validation")
	}
	defer src.Close()

	buffer := make([]byte, 512)
	n, err := io.ReadFull(src, buffer)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return fmt.Errorf("could not read file for validation")
	}
	buffer = buffer[:n]

	contentType := http.DetectContentType(buffer)
	allowedMIMETypes := map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/webp": ".webp",
	}

	ext, ok := allowedMIMETypes[contentType]
	if !ok {
		return fmt.Errorf("invalid file format. Only JPG, PNG, and WEBP are allowed")
	}

	providedExt := strings.ToLower(filepath.Ext(file.Filename))
	if providedExt != ext && !(ext == ".jpg" && providedExt == ".jpeg") {
		return fmt.Errorf("file extension %s does not match the actual content type %s", providedExt, contentType)
	}

	return nil
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

	if err := validateAvatar(file); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext == ".jpeg" {
		ext = ".jpg"
	}

	if user.AvatarURL != "" {
		oldAvatarPath := filepath.Join(h.cfg.Storage.UserAvatar, filepath.Base(user.AvatarURL))
		_ = os.Remove(oldAvatarPath)
	}

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
