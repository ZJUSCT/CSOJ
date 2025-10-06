package admin

import (
	"github.com/ZJUSCT/CSOJ/internal/api"
	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/judger"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NewAdminRouter creates and configures the admin Gin engine.
func NewAdminRouter(
	cfg *config.Config,
	db *gorm.DB,
	scheduler *judger.Scheduler,
	appState *judger.AppState) *gin.Engine {

	r := gin.Default()

	r.Use(api.CORSMiddleware(cfg.CORS))

	h := NewHandler(cfg, db, scheduler, appState)

	// Websocket
	r.GET("/ws/submissions/:id/containers/:conID/logs", h.handleAdminContainerWs)

	// Management
	r.POST("/reload", h.reload)

	// User Management
	users := r.Group("/users")
	{
		users.GET("", h.getAllUsers)
		users.POST("", h.createUser)
		users.DELETE("/:id", h.deleteUser)
	}

	// Submission Management
	submissions := r.Group("/submissions")
	{
		submissions.GET("", h.getAllSubmissions)
		submissions.GET("/:id", h.getSubmission)
		submissions.GET("/:id/containers/:conID/log", h.getContainerLog)
		submissions.POST("/:id/rejudge", h.rejudgeSubmission)
		submissions.PATCH("/:id/validity", h.updateSubmissionValidity)
		submissions.POST("/:id/interrupt", h.interruptSubmission)
	}

	// Cluster Management
	r.GET("/clusters/status", h.getClusterStatus)

	return r
}
