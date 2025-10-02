package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/gitlab"
	"gorm.io/gorm"
)

type GitLabHandler struct {
	cfg    *config.Config
	db     *gorm.DB
	oauth2 *oauth2.Config
}

type GitLabUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar_url"`
}

func NewGitLabHandler(cfg *config.Config, db *gorm.DB) *GitLabHandler {
	return &GitLabHandler{
		cfg: cfg,
		db:  db,
		oauth2: &oauth2.Config{
			ClientID:     cfg.Auth.GitLab.ClientID,
			ClientSecret: cfg.Auth.GitLab.ClientSecret,
			RedirectURL:  cfg.Auth.GitLab.RedirectURI,
			Endpoint:     gitlab.Endpoint,
			Scopes:       []string{"read_user"},
		},
	}
}

func (h *GitLabHandler) Login(c *gin.Context) {
	url := h.oauth2.AuthCodeURL("state", oauth2.AccessTypeOffline)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func (h *GitLabHandler) Callback(c *gin.Context) {
	code := c.Query("code")
	token, err := h.oauth2.Exchange(context.Background(), code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to exchange token: " + err.Error()})
		return
	}

	client := h.oauth2.Client(context.Background(), token)
	resp, err := client.Get(h.cfg.Auth.GitLab.URL + "/api/v4/user")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user info: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	var gitlabUser GitLabUser
	if err := json.NewDecoder(resp.Body).Decode(&gitlabUser); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode user info: " + err.Error()})
		return
	}

	user, err := database.GetUserByGitLabID(h.db, fmt.Sprint(gitlabUser.ID))
	if err == gorm.ErrRecordNotFound {
		// User does not exist, create new user
		newUser := models.User{
			ID:        uuid.New().String(),
			GitLabID:  fmt.Sprint(gitlabUser.ID),
			Username:  gitlabUser.Username,
			Nickname:  gitlabUser.Name,
			AvatarURL: gitlabUser.Avatar,
		}
		if err := database.CreateUser(h.db, &newUser); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user: " + err.Error()})
			return
		}
		user = &newUser
		zap.S().Infof("new user registered: %s", user.Username)
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error: " + err.Error()})
		return
	}

	jwtToken, err := GenerateJWT(user.GitLabID, h.cfg.Auth.JWT.Secret, h.cfg.Auth.JWT.ExpireHours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate JWT: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": jwtToken})
}
