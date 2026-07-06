package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

type GitLabHandler struct {
	cfg      *config.Config
	settings *config.SettingsStore
	db       *gorm.DB
}

type OIDCClaims struct {
	PreferredUsername string `json:"preferred_username"`
	Name              string `json:"name"`
	Picture           string `json:"picture"`
}

func NewGitLabHandler(cfg *config.Config, settings *config.SettingsStore, db *gorm.DB) *GitLabHandler {
	return &GitLabHandler{cfg: cfg, settings: settings, db: db}
}

// gitlabSettings reads the auth.gitlab settings row.
type gitlabSettings struct {
	App                 string `json:"app"`
	URL                 string `json:"url"`
	ClientID            string `json:"client_id"`
	ClientSecret        string `json:"client_secret"`
	RedirectURI         string `json:"redirect_uri"`
	FrontendCallbackURL string `json:"frontend_callback_url"`
}

func (h *GitLabHandler) loadSettings(c *gin.Context) (*gitlabSettings, error) {
	var gl gitlabSettings
	if err := h.settings.Get("auth.gitlab", &gl); err != nil {
		return nil, fmt.Errorf("read gitlab settings: %w", err)
	}
	if gl.URL == "" || gl.ClientID == "" || gl.ClientSecret == "" || gl.RedirectURI == "" {
		return nil, errors.New("gitlab not configured")
	}
	return &gl, nil
}

// buildProvider constructs the OIDC provider + oauth2 config for this request.
func (h *GitLabHandler) buildProvider(c *gin.Context) (*oidc.Provider, *oauth2.Config, *oidc.IDTokenVerifier, *gitlabSettings, error) {
	gl, err := h.loadSettings(c)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	provider, err := oidc.NewProvider(c.Request.Context(), gl.URL)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("create OIDC provider: %w", err)
	}
	oauth2Config := &oauth2.Config{
		ClientID:     gl.ClientID,
		ClientSecret: gl.ClientSecret,
		RedirectURL:  gl.RedirectURI,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID},
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: gl.ClientID})
	return provider, oauth2Config, verifier, gl, nil
}

func (h *GitLabHandler) jwtExpireHours() int {
	var n int
	if err := h.settings.Get("auth.jwt.expire_hours", &n); err != nil || n <= 0 {
		return h.cfg.Auth.JWT.ExpireHours
	}
	return n
}

func (h *GitLabHandler) Login(c *gin.Context) {
	_, oauth2Config, _, _, err := h.buildProvider(c)
	if err != nil {
		util.Error(c, http.StatusServiceUnavailable, err)
		return
	}
	url := oauth2Config.AuthCodeURL("state")
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func (h *GitLabHandler) Callback(c *gin.Context) {
	ctx := c.Request.Context()
	code := c.Query("code")

	_, oauth2Config, verifier, gl, err := h.buildProvider(c)
	if err != nil {
		util.Error(c, http.StatusServiceUnavailable, err)
		return
	}

	frontendURL := gl.FrontendCallbackURL
	if frontendURL == "" {
		frontendURL = "/callback"
		zap.S().Warnf("frontend_callback_url not set in settings, using default: %s", frontendURL)
	}

	redirectURL := frontendURL

	if !strings.Contains(frontendURL, "?") {
		frontendURL += "?"
	} else {
		frontendURL += "&"
	}
	frontendURL += "error="

	token, err := oauth2Config.Exchange(ctx, code)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, frontendURL+"token_exchange_failed")
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		c.Redirect(http.StatusTemporaryRedirect, frontendURL+"id_token_missing")
		return
	}

	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, frontendURL+"id_token_verification_failed")
		return
	}

	var claims OIDCClaims
	if err := idToken.Claims(&claims); err != nil {
		c.Redirect(http.StatusTemporaryRedirect, frontendURL+"claims_extraction_failed")
		return
	}

	gitlabIDStr := idToken.Subject
	user, err := database.GetUserByGitLabID(h.db, gitlabIDStr)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if claims.PreferredUsername == "" {
			c.Redirect(http.StatusTemporaryRedirect, frontendURL+"username_claim_missing")
			return
		}
		// Also check if the username already exists from a local account
		_, err := database.GetUserByUsername(h.db, claims.PreferredUsername)
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			if err == nil {
				c.Redirect(http.StatusTemporaryRedirect, frontendURL+"username_already_exists")
			} else {
				c.Redirect(http.StatusTemporaryRedirect, frontendURL+"database_error")
			}
			return
		}

		newUser := models.User{
			ID:        uuid.New().String(),
			GitLabID:  &gitlabIDStr,
			Username:  claims.PreferredUsername,
			Nickname:  claims.Name,
			AvatarURL: claims.Picture,
		}
		// Bootstrap: the first registered user becomes superadmin.
		// Known race: two concurrent first-registrations could both see count==0;
		// acceptable for bootstrap (one-time event on a fresh DB).
		count, err := database.CountUsers(h.db)
		if err != nil {
			c.Redirect(http.StatusTemporaryRedirect, frontendURL+"database_error")
			return
		}
		if count == 0 {
			newUser.Role = models.RoleSuperAdmin
			zap.S().Infof("first user registered (%s); granting superadmin", newUser.Username)
		}
		if err := database.CreateUser(h.db, &newUser); err != nil {
			c.Redirect(http.StatusTemporaryRedirect, frontendURL+"user_creation_failed")
			return
		}
		user = &newUser
		zap.S().Infof("new OIDC user registered: %s", user.Username)
	} else if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, frontendURL+"database_error")
		return
	}

	jwtToken, err := GenerateJWT(user.ID, string(user.Role), h.cfg.Auth.JWT.Secret, h.jwtExpireHours())
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, frontendURL+"jwt_generation_failed")
		return
	}

	if !strings.Contains(redirectURL, "?") {
		redirectURL += "?"
	} else {
		redirectURL += "&"
	}
	redirectURL += "token=" + jwtToken

	c.Redirect(http.StatusTemporaryRedirect, redirectURL)
}
