package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

type GitLabHandler struct {
	cfg      *config.Config
	db       *gorm.DB
	oauth2   *oauth2.Config
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
}

type OIDCClaims struct {
	PreferredUsername string `json:"preferred_username"`
	Name              string `json:"name"`
	Picture           string `json:"picture"`
}

func NewGitLabHandler(cfg *config.Config, db *gorm.DB) *GitLabHandler {
	ctx := context.Background()

	provider, err := oidc.NewProvider(ctx, cfg.Auth.GitLab.URL)
	if err != nil {
		zap.S().Fatalf("failed to create OIDC provider: %v", err)
	}

	oauth2Config := &oauth2.Config{
		ClientID:     cfg.Auth.GitLab.ClientID,
		ClientSecret: cfg.Auth.GitLab.ClientSecret,
		RedirectURL:  cfg.Auth.GitLab.RedirectURI,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile"},
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.Auth.GitLab.ClientID})

	return &GitLabHandler{
		cfg:      cfg,
		db:       db,
		oauth2:   oauth2Config,
		provider: provider,
		verifier: verifier,
	}
}

func (h *GitLabHandler) Login(c *gin.Context) {
	url := h.oauth2.AuthCodeURL("state")
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func (h *GitLabHandler) Callback(c *gin.Context) {
	ctx := c.Request.Context()
	code := c.Query("code")
	token, err := h.oauth2.Exchange(ctx, code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to exchange token: " + err.Error()})
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "id_token not found in oauth2 token"})
		return
	}

	idToken, err := h.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Failed to verify id_token: " + err.Error()})
		return
	}

	var claims OIDCClaims
	if err := idToken.Claims(&claims); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to extract claims from id_token: " + err.Error()})
		return
	}

	gitlabIDStr := idToken.Subject

	user, err := database.GetUserByGitLabID(h.db, gitlabIDStr)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if claims.PreferredUsername == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "preferred_username claim not found in id_token"})
			return
		}
		newUser := models.User{
			ID:        uuid.New().String(),
			GitLabID:  &gitlabIDStr,
			Username:  claims.PreferredUsername,
			Nickname:  claims.Name,
			AvatarURL: claims.Picture,
		}
		if err := database.CreateUser(h.db, &newUser); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user: " + err.Error()})
			return
		}
		user = &newUser
		zap.S().Infof("new OIDC user registered: %s", user.Username)
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error: " + err.Error()})
		return
	} else {
		// 更新用户信息
		shouldUpdate := false
		if user.Nickname != claims.Name {
			user.Nickname = claims.Name
			shouldUpdate = true
		}
		if user.AvatarURL != claims.Picture {
			user.AvatarURL = claims.Picture
			shouldUpdate = true
		}
		if shouldUpdate {
			if err := database.UpdateUser(h.db, user); err != nil {
				zap.S().Warnf("failed to update user info for %s: %v", user.Username, err)
			}
		}
	}

	jwtToken, err := GenerateJWT(user.ID, h.cfg.Auth.JWT.Secret, h.cfg.Auth.JWT.ExpireHours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate JWT: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": jwtToken})
}
