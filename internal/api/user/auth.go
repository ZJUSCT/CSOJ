package user

import (
	"errors"
	"net/http"

	"github.com/ZJUSCT/CSOJ/internal/auth"
	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func (h *Handler) getAuthStatus(c *gin.Context) {
	util.Success(c, gin.H{
		"local_auth_enabled": h.cfg.Auth.Local.Enabled,
	}, "Auth status retrieved")
}

func (h *Handler) localRegister(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		Nickname string `json:"nickname"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	_, err := database.GetUserByUsername(h.db, req.Username)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		if err == nil {
			util.Error(c, http.StatusConflict, "username already exists")
		} else {
			util.Error(c, http.StatusInternalServerError, "database error")
		}
		return
	}

	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, "failed to hash password")
		return
	}

	newUser := models.User{
		ID:           uuid.NewString(),
		Username:     req.Username,
		PasswordHash: hashedPassword,
		Nickname:     req.Nickname,
	}
	if newUser.Nickname == "" {
		newUser.Nickname = newUser.Username
	}

	if err := database.CreateUser(h.db, &newUser); err != nil {
		util.Error(c, http.StatusInternalServerError, "failed to create user")
		return
	}

	zap.S().Infof("new local user registered: %s", newUser.Username)
	util.Success(c, gin.H{"id": newUser.ID, "username": newUser.Username}, "User registered successfully")
}

func (h *Handler) localLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	user, err := database.GetUserByUsername(h.db, req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			util.Error(c, http.StatusUnauthorized, "invalid username or password")
		} else {
			util.Error(c, http.StatusInternalServerError, "database error")
		}
		return
	}

	if user.PasswordHash == "" {
		util.Error(c, http.StatusUnauthorized, "user registered via GitLab, please use GitLab login")
		return
	}

	if !auth.CheckPasswordHash(req.Password, user.PasswordHash) {
		util.Error(c, http.StatusUnauthorized, "invalid username or password")
		return
	}

	jwtToken, err := auth.GenerateJWT(user.ID, h.cfg.Auth.JWT.Secret, h.cfg.Auth.JWT.ExpireHours)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, "failed to generate JWT")
		return
	}
	util.Success(c, gin.H{"token": jwtToken}, "Login successful")
}
