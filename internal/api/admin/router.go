package admin

import (
	"github.com/ZJUSCT/CSOJ/internal/api"
	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/embedui"
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

	v1 := r.Group("/api/v1")
	{
		// Websocket
		v1.GET("/ws/submissions/:id/containers/:conID/logs", h.handleAdminContainerWs)

		// Management
		v1.POST("/reload", h.reload)

		// User Management
		users := v1.Group("/users")
		{
			users.GET("", h.getAllUsers)
			users.POST("", h.createUser)
			users.GET("/:id", h.getUser)
			users.PATCH("/:id", h.updateUser)
			users.DELETE("/:id", h.deleteUser)
			users.GET("/:id/history", h.getUserContestHistory)
			users.POST("/:id/reset-password", h.resetUserPassword)
			users.POST("/:id/register-contest", h.registerUserForContest)
			users.GET("/:id/scores", h.getUserScores)
		}

		// Submission Management
		submissions := v1.Group("/submissions")
		{
			submissions.GET("", h.getAllSubmissions)
			submissions.GET("/:id", h.getSubmission)
			submissions.GET("/:id/content", h.getSubmissionContent)
			submissions.PATCH("/:id", h.updateSubmission)
			submissions.DELETE("/:id", h.deleteSubmission)
			submissions.GET("/:id/containers/:conID/log", h.getContainerLog)
			submissions.POST("/:id/rejudge", h.rejudgeSubmission)
			submissions.PATCH("/:id/validity", h.updateSubmissionValidity)
			submissions.POST("/:id/interrupt", h.interruptSubmission)
		}

		// Contest & Problem Management
		contests := v1.Group("/contests")
		{
			contests.GET("", h.getAllContests)
			contests.POST("", h.createContest)
			contests.GET("/:id", h.getContest)
			contests.PUT("/:id", h.updateContest)
			contests.DELETE("/:id", h.deleteContest)
			contests.GET("/:id/leaderboard", h.getContestLeaderboard)
			contests.GET("/:id/trend", h.getContestTrend)
			contests.POST("/:id/problems", h.createProblemInContest)
			// Contest Assets
			contests.GET("/:id/assets", h.handleListContestAssets)
			contests.POST("/:id/assets", h.handleUploadContestAssets)
			contests.DELETE("/:id/assets", h.handleDeleteContestAsset)
		}

		problems := v1.Group("/problems")
		{
			problems.GET("", h.getAllProblems)
			problems.GET("/:id", h.getProblem)
			problems.PUT("/:id", h.updateProblem)
			problems.DELETE("/:id", h.deleteProblem)
			// Problem Assets
			problems.GET("/:id/assets", h.handleListProblemAssets)
			problems.POST("/:id/assets", h.handleUploadProblemAssets)
			problems.DELETE("/:id/assets", h.handleDeleteProblemAsset)
		}

		// Score Management
		scores := v1.Group("/scores")
		{
			scores.POST("/recalculate", h.recalculateScore)
		}

		// Cluster Management
		clusters := v1.Group("/clusters")
		{
			clusters.GET("/status", h.getClusterStatus)
			clusters.GET("/:clusterName/nodes/:nodeName", h.getNodeDetails)
			clusters.POST("/:clusterName/nodes/:nodeName/pause", h.pauseNode)
			clusters.POST("/:clusterName/nodes/:nodeName/resume", h.resumeNode)
		}

		// Container Management
		containers := v1.Group("/containers")
		{
			containers.GET("", h.getAllContainers)
			containers.GET("/:id", h.getContainer)
		}
	}

	embedui.RegisterUIHandlers(r, "admin")

	return r
}
