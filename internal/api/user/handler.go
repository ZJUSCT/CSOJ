package user

import (
	"github.com/ZJUSCT/CSOJ/internal/auth"
	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/judger"
	"gorm.io/gorm"
)

// Handler holds all dependencies for the user API handlers.
type Handler struct {
	cfg               *config.Config
	db                *gorm.DB
	scheduler         *judger.Scheduler
	appState          *judger.AppState
	gitlabAuthHandler *auth.GitLabHandler
}

// NewHandler creates a new user handler with its dependencies.
func NewHandler(
	cfg *config.Config,
	db *gorm.DB,
	scheduler *judger.Scheduler,
	appState *judger.AppState,
) *Handler {
	return &Handler{
		cfg:               cfg,
		db:                db,
		scheduler:         scheduler,
		appState:          appState,
		gitlabAuthHandler: auth.NewGitLabHandler(cfg, db),
	}
}
