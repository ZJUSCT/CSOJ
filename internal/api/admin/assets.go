package admin

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AssetInfo struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"` // Relative to index.assets
	IsDir   bool      `json:"is_dir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// listAssets safely lists all files and directories within a given asset root.
func listAssets(assetsRoot string) ([]AssetInfo, error) {
	if _, err := os.Stat(assetsRoot); os.IsNotExist(err) {
		// If the assets directory doesn't exist, return an empty list.
		return []AssetInfo{}, nil
	}

	var assets []AssetInfo
	err := filepath.WalkDir(assetsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory itself.
		if path == assetsRoot {
			return nil
		}

		relPath, err := filepath.Rel(assetsRoot, path)
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		assets = append(assets, AssetInfo{
			Name:    d.Name(),
			Path:    filepath.ToSlash(relPath), // Use forward slashes for consistency
			IsDir:   d.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
		return nil
	})

	if err != nil {
		return nil, err
	}
	return assets, nil
}

// getSafeAssetPath is a security helper to ensure operations are within the allowed directory.
func getSafeAssetPath(basePath, userPath string) (string, error) {
	assetsRoot := filepath.Join(basePath, "index.assets")
	safeAssetsRoot, err := filepath.Abs(assetsRoot)
	if err != nil {
		return "", fmt.Errorf("could not get absolute path for asset root: %w", err)
	}

	// Clean the user-provided path to resolve ".." etc.
	cleanedUserPath := filepath.Clean(userPath)

	// Join with the root
	finalPath := filepath.Join(safeAssetsRoot, cleanedUserPath)
	safeFinalPath, err := filepath.Abs(finalPath)
	if err != nil {
		return "", fmt.Errorf("could not get absolute path for final path: %w", err)
	}

	// The final check: the resulting absolute path must have the asset root as a prefix.
	if !strings.HasPrefix(safeFinalPath, safeAssetsRoot) {
		return "", fmt.Errorf("path traversal attempt detected")
	}
	return safeFinalPath, nil
}

// handleListContestAssets lists assets for a contest.
func (h *Handler) handleListContestAssets(c *gin.Context) {
	contestID := c.Param("id")
	h.appState.RLock()
	contest, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	assets, err := listAssets(filepath.Join(contest.BasePath, "index.assets"))
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to list assets: %w", err))
		return
	}
	util.Success(c, assets, "Assets listed successfully")
}

// handleListProblemAssets lists assets for a problem.
func (h *Handler) handleListProblemAssets(c *gin.Context) {
	problemID := c.Param("id")
	h.appState.RLock()
	problem, ok := h.appState.Problems[problemID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}

	assets, err := listAssets(filepath.Join(problem.BasePath, "index.assets"))
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to list assets: %w", err))
		return
	}
	util.Success(c, assets, "Assets listed successfully")
}

// handleUploadAsset is a generic handler for uploading assets.
func (h *Handler) handleUploadAsset(c *gin.Context, basePath string) {
	form, err := c.MultipartForm()
	if err != nil {
		util.Error(c, http.StatusBadRequest, fmt.Errorf("failed to parse multipart form: %w", err))
		return
	}

	files := form.File["files"]
	relativePath := form.Value["path"] // Optional subdirectory path

	for _, file := range files {
		// Construct the destination path safely
		destRelPath := filepath.Join(append(relativePath, file.Filename)...)
		destAbsPath, err := getSafeAssetPath(basePath, destRelPath)
		if err != nil {
			util.Error(c, http.StatusBadRequest, err)
			return
		}

		// Create subdirectory if it doesn't exist
		if err := os.MkdirAll(filepath.Dir(destAbsPath), 0755); err != nil {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to create directory: %w", err))
			return
		}

		// Save the uploaded file
		if err := c.SaveUploadedFile(file, destAbsPath); err != nil {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to save file: %w", err))
			return
		}
	}

	util.Success(c, gin.H{"files_uploaded": len(files)}, "Files uploaded successfully")
}

// handleUploadContestAssets uploads assets for a contest.
func (h *Handler) handleUploadContestAssets(c *gin.Context) {
	contestID := c.Param("id")
	h.appState.RLock()
	contest, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}
	h.handleUploadAsset(c, contest.BasePath)
}

// handleUploadProblemAssets uploads assets for a problem.
func (h *Handler) handleUploadProblemAssets(c *gin.Context) {
	problemID := c.Param("id")
	h.appState.RLock()
	problem, ok := h.appState.Problems[problemID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}
	h.handleUploadAsset(c, problem.BasePath)
}

// handleDeleteAsset is a generic handler for deleting an asset.
func (h *Handler) handleDeleteAsset(c *gin.Context, basePath string) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	targetPath, err := getSafeAssetPath(basePath, req.Path)
	if err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	if err := os.RemoveAll(targetPath); err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to delete asset: %w", err))
		return
	}
	zap.S().Warnf("admin deleted asset at '%s'", req.Path)
	util.Success(c, nil, "Asset deleted successfully")
}

// handleDeleteContestAsset deletes an asset from a contest.
func (h *Handler) handleDeleteContestAsset(c *gin.Context) {
	contestID := c.Param("id")
	h.appState.RLock()
	contest, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}
	h.handleDeleteAsset(c, contest.BasePath)
}

// handleDeleteProblemAsset deletes an asset from a problem.
func (h *Handler) handleDeleteProblemAsset(c *gin.Context) {
	problemID := c.Param("id")
	h.appState.RLock()
	problem, ok := h.appState.Problems[problemID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}
	h.handleDeleteAsset(c, problem.BasePath)
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
