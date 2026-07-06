package user

import (
	"crypto/hmac"
	"crypto/sha512"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)

func (h *Handler) serveAvatar(c *gin.Context) {
	filename := c.Param("filename")
	// Basic security: prevent path traversal
	cleanFilename := filepath.Base(filename)
	if cleanFilename != filename {
		util.Error(c, http.StatusBadRequest, "invalid filename")
		return
	}

	fullPath := filepath.Join(h.cfg.Storage.UserAvatar, cleanFilename)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		util.Error(c, http.StatusNotFound, "avatar not found")
		return
	}
	c.File(fullPath)
}

func (h *Handler) queryAssetURL(c *gin.Context) {
	asset := c.Query("asset")

	if !strings.HasPrefix(asset, "/api/v1/assets/") {
		util.Error(c, http.StatusBadRequest, "invalid asset path")
		return
	}

	timeout := time.Now().Add(15 * time.Minute).Unix()

	message := fmt.Sprintf("%s|%d", asset, timeout)

	mac := hmac.New(sha512.New, []byte(h.cfg.Auth.JWT.Secret))
	mac.Write([]byte(message))
	token := fmt.Sprintf("%x", mac.Sum(nil))

	signedURL := fmt.Sprintf("%s?token=%s&expires=%d", asset, token, timeout)

	util.Success(c, gin.H{"url": signedURL}, "Asset URL generated")
}

func (h *Handler) serveContestAsset(c *gin.Context) {
	contestID := c.Param("id")
	assetPath := c.Param("assetpath")

	h.appState.RLock()
	_, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	var row models.Asset
	if err := h.db.Where("owner_type = ? AND owner_id = ? AND path = ?", "contest", contestID, cleanUserAssetPath(assetPath)).First(&row).Error; err != nil {
		util.Error(c, http.StatusNotFound, "asset not found")
		return
	}
	if row.IsDir {
		util.Error(c, http.StatusBadRequest, "cannot serve a directory")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", path.Base(row.Path)))
	c.Data(http.StatusOK, contentTypeFor(row.Path), row.Content)
}

func (h *Handler) serveProblemAsset(c *gin.Context) {
	problemID := c.Param("id")
	assetPath := c.Param("assetpath")

	h.appState.RLock()
	problem, ok := h.appState.Problems[problemID]
	if !ok {
		h.appState.RUnlock()
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}
	parentContest, ok := h.appState.ProblemToContestMap[problemID]
	if !ok {
		h.appState.RUnlock()
		util.Error(c, http.StatusInternalServerError, "internal server error: problem has no parent contest")
		return
	}
	now := time.Now()
	if now.Before(parentContest.StartTime) {
		h.appState.RUnlock()
		util.Error(c, http.StatusForbidden, "contest has not started yet")
		return
	}
	if now.Before(problem.StartTime) {
		h.appState.RUnlock()
		util.Error(c, http.StatusForbidden, "problem has not started yet")
		return
	}
	h.appState.RUnlock()

	var row models.Asset
	if err := h.db.Where("owner_type = ? AND owner_id = ? AND path = ?", "problem", problemID, cleanUserAssetPath(assetPath)).First(&row).Error; err != nil {
		util.Error(c, http.StatusNotFound, "asset not found")
		return
	}
	if row.IsDir {
		util.Error(c, http.StatusBadRequest, "cannot serve a directory")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", path.Base(row.Path)))
	c.Data(http.StatusOK, contentTypeFor(row.Path), row.Content)
}

func cleanUserAssetPath(p string) string {
	p = strings.TrimPrefix(p, "/")
	cleaned := path.Clean(p)
	if strings.HasPrefix(cleaned, "..") {
		return ""
	}
	return strings.ReplaceAll(cleaned, "\\", "/")
}

func contentTypeFor(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".md":
		return "text/markdown; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".json":
		return "application/json"
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}
