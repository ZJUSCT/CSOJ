package user

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// PublicProfileResponse 定义了用户的公开可访问信息。
type PublicProfileResponse struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	Signature string `json:"signature"`
	AvatarURL string `json:"avatar_url"`
	Tags      string `json:"tags"`
}

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

func (h *Handler) getPublicUserProfile(c *gin.Context) {
	userID := c.Param("id")
	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			util.Error(c, http.StatusNotFound, "user not found")
		} else {
			util.Error(c, http.StatusInternalServerError, "database error")
		}
		return
	}

	avatarURL := user.AvatarURL
	if user.AvatarURL != "" && !strings.HasPrefix(user.AvatarURL, "http") {
		avatarURL = fmt.Sprintf("/api/v1/assets/avatars/%s", user.AvatarURL)
	}

	response := PublicProfileResponse{
		ID:        user.ID,
		Username:  user.Username,
		Nickname:  user.Nickname,
		Signature: user.Signature,
		AvatarURL: avatarURL,
		Tags:      user.Tags,
	}

	util.Success(c, response, "User profile retrieved successfully")
}

var maliciousContentRegex = regexp.MustCompile("<[^>]+>")

func containsMaliciousContent(s string) bool {
	// Check for "javascript:"
	if strings.Contains(strings.ToLower(s), "javascript:") {
		return true
	}
	// Check for HTML tags
	if maliciousContentRegex.MatchString(s) {
		return true
	}
	return false
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

	if containsMaliciousContent(reqBody.Nickname) || containsMaliciousContent(reqBody.Signature) {
		banUntil := time.Now().Add(24 * time.Hour)
		user.BannedUntil = &banUntil
		user.BanReason = "Hacking Detected"
		if err := database.UpdateUser(h.db, user); err != nil {
			util.Error(c, http.StatusInternalServerError, err)
			return
		}
		zap.S().Warnf("user %s (%s) auto-banned for 24 hours due to suspicious nickname/signature", user.Username, user.ID)
		util.Error(c, http.StatusForbidden, "Your account has been temporarily banned due to suspicious input.")
		return
	}

	if len(reqBody.Nickname) == 0 || len(reqBody.Nickname) > 15 {
		util.Error(c, http.StatusBadRequest, "nickname must be between 1 and 15 characters")
		return
	}
	for _, char := range "{}|[]\\:\";'<>?,./" {
		if strings.ContainsRune(reqBody.Nickname, char) {
			util.Error(c, http.StatusBadRequest, "nickname not allowed")
			return
		}
	}
	if len(reqBody.Signature) > 100 {
		util.Error(c, http.StatusBadRequest, "signature must be at most 100 characters")
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
	const maxAvatarSize = 1024 * 1024
	if file.Size > maxAvatarSize {
		return fmt.Errorf("avatar file is too large. Maximum size is 1MB")
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
