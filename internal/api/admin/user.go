package admin

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/auth"
	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func (h *Handler) getAllUsers(c *gin.Context) {
	searchQuery := c.Query("query")
	dbQuery := h.db

	if searchQuery != "" {
		likeQuery := "%" + searchQuery + "%"
		dbQuery = dbQuery.Where("id = ? OR username LIKE ? OR nickname LIKE ?", searchQuery, likeQuery, likeQuery)
	}

	var users []models.User
	if err := dbQuery.Find(&users).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	util.Success(c, users, "Users retrieved successfully")
}

func (h *Handler) getUser(c *gin.Context) {
	userID := c.Param("id")
	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			util.Error(c, http.StatusNotFound, "user not found")
		} else {
			util.Error(c, http.StatusInternalServerError, err)
		}
		return
	}
	util.Success(c, user, "User retrieved successfully")
}

func (h *Handler) updateUser(c *gin.Context) {
	userID := c.Param("id")
	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			util.Error(c, http.StatusNotFound, "user not found")
		} else {
			util.Error(c, http.StatusInternalServerError, err)
		}
		return
	}

	var reqBody struct {
		Nickname    *string `json:"nickname"`
		Signature   *string `json:"signature"`
		BanReason   *string `json:"ban_reason"`
		BannedUntil *string `json:"banned_until"` // Receive as string to handle null/empty
		DisableRank *bool   `json:"disable_rank"`
		Tags        *string `json:"tags"`
	}

	if err := c.ShouldBindJSON(&reqBody); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	if reqBody.Nickname != nil {
		user.Nickname = *reqBody.Nickname
	}
	if reqBody.Signature != nil {
		user.Signature = *reqBody.Signature
	}
	if reqBody.DisableRank != nil {
		user.DisableRank = *reqBody.DisableRank
	}
	if reqBody.Tags != nil {
		user.Tags = *reqBody.Tags // Store as comma-separated string
	}

	// Handle ban logic
	if reqBody.BanReason != nil {
		user.BanReason = *reqBody.BanReason
	}
	if reqBody.BannedUntil != nil {
		if *reqBody.BannedUntil == "" {
			user.BannedUntil = nil // Unban by sending empty string
			user.BanReason = ""    // Clear reason on unban
		} else {
			// Parse the time string. `time.RFC3339` is the standard for JS `toISOString()`
			t, err := time.Parse(time.RFC3339, *reqBody.BannedUntil)
			if err != nil {
				// Fallback for HTML datetime-local input which doesn't include timezone
				t, err = time.Parse("2006-01-02T15:04", *reqBody.BannedUntil)
				if err != nil {
					util.Error(c, http.StatusBadRequest, "invalid banned_until time format")
					return
				}
			}
			user.BannedUntil = &t
		}
	}

	if err := database.UpdateUser(h.db, user); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, user, "User profile updated successfully")
}

func (h *Handler) createUser(c *gin.Context) {
	var user models.User
	if err := c.ShouldBindJSON(&user); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	user.ID = uuid.NewString()
	if err := database.CreateUser(h.db, &user); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, user, "User created successfully")
}

func (h *Handler) deleteUser(c *gin.Context) {
	userID := c.Param("id")
	if err := database.DeleteUser(h.db, userID); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, nil, "User deleted successfully")
}

func (h *Handler) getUserContestHistory(c *gin.Context) {
	userID := c.Param("id")
	contestID := c.Query("contest_id")

	if contestID == "" {
		util.Error(c, http.StatusBadRequest, "contest_id query parameter is required")
		return
	}

	if _, err := database.GetUserByID(h.db, userID); err != nil {
		util.Error(c, http.StatusNotFound, "user not found")
		return
	}
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

func (h *Handler) resetUserPassword(c *gin.Context) {
	userID := c.Param("id")
	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		util.Error(c, http.StatusNotFound, "user not found")
		return
	}

	if user.GitLabID != nil {
		util.Error(c, http.StatusBadRequest, "cannot reset password for GitLab user")
		return
	}

	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, "failed to hash new password")
		return
	}

	user.PasswordHash = hashedPassword
	if err := database.UpdateUser(h.db, user); err != nil {
		util.Error(c, http.StatusInternalServerError, "failed to update user password")
		return
	}

	zap.S().Warnf("admin reset password for user %s (%s)", user.Username, user.ID)
	util.Success(c, nil, "User password reset successfully")
}

func (h *Handler) registerUserForContest(c *gin.Context) {
	userID := c.Param("id")
	var req struct {
		ContestID string `json:"contest_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	if _, err := database.GetUserByID(h.db, userID); err != nil {
		util.Error(c, http.StatusNotFound, "user not found")
		return
	}
	h.appState.RLock()
	_, ok := h.appState.Contests[req.ContestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	if err := database.RegisterForContest(h.db, userID, req.ContestID); err != nil {
		if err.Error() == "already registered" {
			util.Error(c, http.StatusConflict, err)
			return
		}
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	zap.S().Infof("admin registered user %s for contest %s", userID, req.ContestID)
	util.Success(c, nil, "Successfully registered user for contest")
}

func (h *Handler) getUserScores(c *gin.Context) {
	userID := c.Param("id")
	if _, err := database.GetUserByID(h.db, userID); err != nil {
		util.Error(c, http.StatusNotFound, "user not found")
		return
	}

	scores, err := database.GetBestScoresByUserID(h.db, userID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	util.Success(c, scores, "User best scores retrieved successfully")
}

func (h *Handler) handleDownloadSolutions(c *gin.Context) {
	userID := c.Param("id")
	contestID := c.Param("contest_id")

	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			util.Error(c, http.StatusNotFound, "user not found")
		} else {
			util.Error(c, http.StatusInternalServerError, err)
		}
		return
	}

	h.appState.RLock()
	contest, ok := h.appState.Contests[contestID]
	if !ok {
		h.appState.RUnlock()
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	problemIDs := make([]string, len(contest.ProblemIDs))
	copy(problemIDs, contest.ProblemIDs)
	h.appState.RUnlock()

	type BestSubmission struct {
		Submission models.Submission
		ProblemID  string
		ProblemIdx int
	}
	var bestSubmissions []BestSubmission

	h.appState.RLock()
	for i, problemID := range problemIDs {
		problem, probOk := h.appState.Problems[problemID]
		if !probOk {
			zap.S().Warnf("Problem %s in contest %s not found in appState, skipping", problemID, contestID)
			continue
		}

		var bestSub models.Submission
		query := h.db.Where("user_id = ? AND problem_id = ? AND is_valid = ?", userID, problemID, true)

		if problem.Score.Mode == "performance" {
			query = query.Order("performance DESC, created_at DESC")
		} else {
			query = query.Order("score DESC, created_at DESC")
		}

		err := query.First(&bestSub).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			h.appState.RUnlock()
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to query best submission for problem %s: %w", problemID, err))
			return
		}

		bestSubmissions = append(bestSubmissions, BestSubmission{
			Submission: bestSub,
			ProblemID:  problemID,
			ProblemIdx: i + 1,
		})
	}
	h.appState.RUnlock()

	if len(bestSubmissions) == 0 {
		util.Error(c, http.StatusNotFound, "no valid submissions found for this user in this contest")
		return
	}

	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	for _, bestSub := range bestSubmissions {
		subID := bestSub.Submission.ID
		submissionPath := filepath.Join(h.cfg.Storage.SubmissionContent, subID)

		info, err := os.Stat(submissionPath)
		if os.IsNotExist(err) || !info.IsDir() {
			zap.S().Warnf("Submission content for %s not found on disk at %s, skipping", subID, submissionPath)
			continue
		}

		zipFolderName := fmt.Sprintf("%d-%s-%s", bestSub.ProblemIdx, bestSub.ProblemID, subID)

		err = filepath.Walk(submissionPath, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}

			relPath, err := filepath.Rel(submissionPath, path)
			if err != nil {
				return err
			}

			header.Name = filepath.Join(zipFolderName, relPath)
			header.Name = filepath.ToSlash(header.Name)
			header.Method = zip.Deflate

			writer, err := zipWriter.CreateHeader(header)
			if err != nil {
				return err
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(writer, file)
			return err
		})

		if err != nil {
			zipWriter.Close()
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to add submission %s to zip: %w", subID, err))
			return
		}
	}

	if err := zipWriter.Close(); err != nil {
		util.Error(c, http.StatusInternalServerError, "failed to finalize zip archive")
		return
	}

	baseFileName := fmt.Sprintf("%s-%s-%s", user.Nickname, user.Username, contestID)

	fallbackFileName := strings.ReplaceAll(baseFileName, "\"", "_")
	fallbackFileName = strings.ReplaceAll(fallbackFileName, ";", "_")
	fallbackFileName = fallbackFileName + ".zip"

	utf8FileName := url.PathEscape(baseFileName + ".zip")
	disposition := fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", fallbackFileName, utf8FileName)

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", disposition)
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}
