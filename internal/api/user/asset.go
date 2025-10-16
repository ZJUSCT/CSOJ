package user

import (
	"crypto/hmac"
	"crypto/sha512"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	contest, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	// Security: ensure the requested path is within the allowed assets directory
	baseAssetDir := filepath.Join(contest.BasePath, "index.assets")
	requestedFile := filepath.Join(contest.BasePath, assetPath)

	safeBase, err := filepath.Abs(baseAssetDir)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, "internal server error")
		return
	}
	safeRequested, err := filepath.Abs(requestedFile)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, "internal server error")
		return
	}

	if !strings.HasPrefix(safeRequested, safeBase) {
		util.Error(c, http.StatusForbidden, "access denied")
		return
	}

	if _, err := os.Stat(safeRequested); os.IsNotExist(err) {
		util.Error(c, http.StatusNotFound, "asset not found")
		return
	}
	c.File(safeRequested)
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

	// --- Authorization Logic (same as GET /problems/:id) ---
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
	// --- End Authorization ---

	// --- Security Logic (same as contest assets) ---
	baseAssetDir := filepath.Join(problem.BasePath, "index.assets")
	requestedFile := filepath.Join(problem.BasePath, assetPath)

	safeBase, err := filepath.Abs(baseAssetDir)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, "internal server error")
		return
	}
	safeRequested, err := filepath.Abs(requestedFile)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, "internal server error")
		return
	}

	if !strings.HasPrefix(safeRequested, safeBase) {
		util.Error(c, http.StatusForbidden, "access denied")
		return
	}

	if _, err := os.Stat(safeRequested); os.IsNotExist(err) {
		util.Error(c, http.StatusNotFound, "asset not found")
		return
	}
	c.File(safeRequested)
}
