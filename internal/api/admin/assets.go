package admin

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AssetInfo struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	IsDir   bool      `json:"is_dir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// listAssetsFromDB lists asset rows for an owner.
func listAssetsFromDB(db *gorm.DB, ownerType, ownerID string) ([]AssetInfo, error) {
	var rows []models.Asset
	if err := db.Find(&rows, "owner_type = ? AND owner_id = ?", ownerType, ownerID).Error; err != nil {
		return nil, err
	}
	out := make([]AssetInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, AssetInfo{
			Name:    path.Base(r.Path),
			Path:    r.Path,
			IsDir:   r.IsDir,
			Size:    r.Size,
			ModTime: r.ModTime,
		})
	}
	return out, nil
}

// cleanAssetPath normalizes a user-supplied relative path and rejects traversal.
func cleanAssetPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "/")
	cleaned := path.Clean(p)
	if cleaned == "." {
		return "", fmt.Errorf("empty asset path")
	}
	if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, "../") {
		return "", fmt.Errorf("path traversal attempt detected")
	}
	return strings.ReplaceAll(cleaned, "\\", "/"), nil
}

// ensureDirRows upserts directory rows for all ancestors of relPath.
func ensureDirRows(db *gorm.DB, ownerType, ownerID, relPath string) error {
	dir := path.Dir(relPath)
	if dir == "." || dir == "/" {
		return nil
	}
	parts := strings.Split(dir, "/")
	current := ""
	now := time.Now()
	for _, p := range parts {
		if p == "" {
			continue
		}
		if current == "" {
			current = p
		} else {
			current = current + "/" + p
		}
		row := models.Asset{
			ID:        uuid.NewString(),
			OwnerType: ownerType,
			OwnerID:   ownerID,
			Path:      current,
			IsDir:     true,
			ModTime:   now,
		}
		if err := db.Where("owner_type = ? AND owner_id = ? AND path = ?",
			ownerType, ownerID, current).Assign(row).FirstOrCreate(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) handleListContestAssets(c *gin.Context) {
	contestID := c.Param("id")
	h.appState.RLock()
	_, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}
	assets, err := listAssetsFromDB(h.db, "contest", contestID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to list assets: %w", err))
		return
	}
	util.Success(c, assets, "Assets listed successfully")
}

func (h *Handler) handleListProblemAssets(c *gin.Context) {
	problemID := c.Param("id")
	h.appState.RLock()
	_, ok := h.appState.Problems[problemID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}
	assets, err := listAssetsFromDB(h.db, "problem", problemID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to list assets: %w", err))
		return
	}
	util.Success(c, assets, "Assets listed successfully")
}

// handleUploadAsset reads multipart files and stores them as Asset rows.
func (h *Handler) handleUploadAsset(c *gin.Context, ownerType, ownerID string) {
	form, err := c.MultipartForm()
	if err != nil {
		util.Error(c, http.StatusBadRequest, fmt.Errorf("failed to parse multipart form: %w", err))
		return
	}
	files := form.File["files"]
	subdir := ""
	if v := form.Value["path"]; len(v) > 0 {
		subdir = v[0]
	}

	count := 0
	now := time.Now()
	for _, file := range files {
		rel, err := cleanAssetPath(strings.TrimPrefix(subdir, "/") + "/" + file.Filename)
		if err != nil {
			util.Error(c, http.StatusBadRequest, err)
			return
		}
		src, err := file.Open()
		if err != nil {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to open uploaded file: %w", err))
			return
		}
		content, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to read uploaded file: %w", err))
			return
		}
		if err := ensureDirRows(h.db, ownerType, ownerID, rel); err != nil {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to create dir rows: %w", err))
			return
		}
		row := models.Asset{
			ID:        uuid.NewString(),
			OwnerType: ownerType,
			OwnerID:   ownerID,
			Path:      rel,
			IsDir:     false,
			Size:      int64(len(content)),
			ModTime:   now,
			Content:   content,
		}
		// Replace any existing row at the same path.
		h.db.Where("owner_type = ? AND owner_id = ? AND path = ?", ownerType, ownerID, rel).Delete(&models.Asset{})
		if err := h.db.Create(&row).Error; err != nil {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to store file: %w", err))
			return
		}
		count++
	}
	util.Success(c, gin.H{"files_uploaded": count}, "Files uploaded successfully")
}

func (h *Handler) handleUploadContestAssets(c *gin.Context) {
	contestID := c.Param("id")
	h.appState.RLock()
	_, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}
	h.handleUploadAsset(c, "contest", contestID)
}

func (h *Handler) handleUploadProblemAssets(c *gin.Context) {
	problemID := c.Param("id")
	h.appState.RLock()
	_, ok := h.appState.Problems[problemID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}
	h.handleUploadAsset(c, "problem", problemID)
}

func (h *Handler) handleDeleteAsset(c *gin.Context, ownerType, ownerID string) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	cleaned, err := cleanAssetPath(req.Path)
	if err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	// Delete the row at this path AND any rows under it (subtree).
	prefix := cleaned + "/"
	res := h.db.Where(
		"owner_type = ? AND owner_id = ? AND (path = ? OR path LIKE ?)",
		ownerType, ownerID, cleaned, prefix+"%",
	).Delete(&models.Asset{})
	if res.Error != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to delete asset: %w", res.Error))
		return
	}
	if res.RowsAffected == 0 {
		util.Error(c, http.StatusNotFound, "asset not found")
		return
	}
	zap.S().Warnf("admin deleted asset at '%s'", cleaned)
	util.Success(c, nil, "Asset deleted successfully")
}

func (h *Handler) handleDeleteContestAsset(c *gin.Context) {
	contestID := c.Param("id")
	h.appState.RLock()
	_, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}
	h.handleDeleteAsset(c, "contest", contestID)
}

func (h *Handler) handleDeleteProblemAsset(c *gin.Context) {
	problemID := c.Param("id")
	h.appState.RLock()
	_, ok := h.appState.Problems[problemID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}
	h.handleDeleteAsset(c, "problem", problemID)
}

// serveAssetDB looks up a single asset row by exact path and writes its bytes.
func (h *Handler) serveAssetDB(c *gin.Context, ownerType, ownerID, assetPath string) {
	cleaned, err := cleanAssetPath(assetPath)
	if err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	var row models.Asset
	if err := h.db.Where("owner_type = ? AND owner_id = ? AND path = ?", ownerType, ownerID, cleaned).First(&row).Error; err != nil {
		util.Error(c, http.StatusNotFound, "asset not found")
		return
	}
	if row.IsDir {
		util.Error(c, http.StatusBadRequest, "cannot serve a directory")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", path.Base(cleaned)))
	c.Data(http.StatusOK, contentTypeFor(cleaned), row.Content)
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
	h.serveAssetDB(c, "contest", contestID, assetPath)
}

func (h *Handler) serveProblemAsset(c *gin.Context) {
	problemID := c.Param("id")
	assetPath := c.Param("assetpath")
	h.appState.RLock()
	_, ok := h.appState.Problems[problemID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}
	h.serveAssetDB(c, "problem", problemID, assetPath)
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
