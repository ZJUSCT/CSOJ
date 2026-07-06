package admin

import (
	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/judger"
	"gorm.io/gorm"
)

// Handler holds all dependencies for the admin API handlers.
type Handler struct {
	cfg       *config.Config
	settings  *config.SettingsStore
	db        *gorm.DB
	scheduler *judger.Scheduler
	appState  *judger.AppState
}

// NewHandler creates a new admin handler with its dependencies.
func NewHandler(
	cfg *config.Config,
	settings *config.SettingsStore,
	db *gorm.DB,
	scheduler *judger.Scheduler,
	appState *judger.AppState,
) *Handler {
	return &Handler{
		cfg:       cfg,
		settings:  settings,
		db:        db,
		scheduler: scheduler,
		appState:  appState,
	}
}
