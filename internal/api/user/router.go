package user

import (
	"github.com/ZJUSCT/CSOJ/internal/api"
	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/embedui"
	"github.com/ZJUSCT/CSOJ/internal/judger"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NewUserRouter creates and configures the user Gin engine.
func NewUserRouter(
	cfg *config.Config,
	db *gorm.DB,
	scheduler *judger.Scheduler,
	appState *judger.AppState) *gin.Engine {

	r := gin.Default()

	r.Use(api.CORSMiddleware(cfg.CORS))

	h := NewHandler(cfg, db, scheduler, appState)

	v1 := r.Group("/api/v1")
	{
		// Auth
		authGroup := v1.Group("/auth")
		{
			authGroup.GET("/status", h.getAuthStatus)
			// GitLab Auth
			gitlabGroup := authGroup.Group("/gitlab")
			gitlabGroup.GET("/login", h.gitlabAuthHandler.Login)
			gitlabGroup.GET("/callback", h.gitlabAuthHandler.Callback)

			// Local Username/Password Auth (if enabled)
			if cfg.Auth.Local.Enabled {
				localAuthGroup := authGroup.Group("/local")
				{
					localAuthGroup.POST("/register", h.localRegister)
					localAuthGroup.POST("/login", h.localLogin)
				}
			}
		}

		// Websocket for container logs with authorization
		v1.GET("/ws/submissions/:subID/containers/:conID/logs", h.handleUserContainerWs)

		// Publicly accessible info
		v1.GET("/links", h.getLinks)
		v1.GET("/contests", h.getAllContests)
		v1.GET("/contests/:id", h.getContest)
		v1.GET("/contests/:id/leaderboard", h.getContestLeaderboard)
		v1.GET("/contests/:id/trend", h.getContestTrend)
		v1.GET("/contests/:id/announcements", h.getContestAnnouncements)
		v1.GET("/problems/:id", h.getProblem)
		v1.GET("/users/:id", h.getPublicUserProfile)

		// Publicly accessible assets
		v1.GET("/assets/avatars/:filename", h.serveAvatar)

		// Authenticated routes
		authed := v1.Group("/")
		authed.Use(api.AuthMiddleware(cfg.Auth.JWT.Secret))
		{
			// User Profile
			profile := authed.Group("/user")
			{
				profile.GET("/profile", h.getUserProfile)
				profile.PATCH("/profile", h.updateUserProfile)
				profile.POST("/avatar", h.uploadAvatar)
			}

			// Contest
			authed.POST("/contests/:id/register", h.registerForContest)
			authed.GET("/contests/:id/history", h.getContestHistory)

			// Problems & Submissions
			authed.POST("/problems/:id/submit", h.submitToProblem)
			authed.GET("/problems/:id/attempts", h.getProblemAttempts)

			submissions := authed.Group("/submissions")
			{
				submissions.GET("", h.getUserSubmissions)
				submissions.GET("/:id", h.getUserSubmission)
				submissions.POST("/:id/interrupt", h.interruptSubmission)
				submissions.GET("/:id/queue_position", h.getSubmissionQueuePosition)
				submissions.GET("/:id/containers/:conID/log", h.getContainerLog)
			}

			// Authenticated assets
			assets := authed.Group("/assets")
			{
				assets.GET("/contests/:id/*assetpath", h.serveContestAsset)
				assets.GET("/problems/:id/*assetpath", h.serveProblemAsset)
			}
		}
	}

	embedui.RegisterUIHandlers(r, "user")

	return r
}
