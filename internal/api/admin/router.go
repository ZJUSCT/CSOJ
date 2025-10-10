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
		users.GET("/:id", h.getUser)
		users.PATCH("/:id", h.updateUser)
		users.DELETE("/:id", h.deleteUser)
		users.GET("/:id/history", h.getUserContestHistory)
		users.POST("/:id/reset-password", h.resetUserPassword)
		users.POST("/:id/register-contest", h.registerUserForContest)
		users.GET("/:id/scores", h.getUserScores)
	}

	// Submission Management
	submissions := r.Group("/submissions")
	{
		submissions.GET("", h.getAllSubmissions)
		submissions.GET("/:id", h.getSubmission)
		submissions.PATCH("/:id", h.updateSubmission)
		submissions.DELETE("/:id", h.deleteSubmission)
		submissions.GET("/:id/containers/:conID/log", h.getContainerLog)
		submissions.POST("/:id/rejudge", h.rejudgeSubmission)
		submissions.PATCH("/:id/validity", h.updateSubmissionValidity)
		submissions.POST("/:id/interrupt", h.interruptSubmission)
	}

	// Contest & Problem Management
	contests := r.Group("/contests")
	{
		contests.GET("", h.getAllContests)
		contests.GET("/:id", h.getContest)
		contests.GET("/:id/leaderboard", h.getContestLeaderboard)
		contests.GET("/:id/trend", h.getContestTrend)
	}

	problems := r.Group("/problems")
	{
		problems.GET("", h.getAllProblems)
		problems.GET("/:id", h.getProblem)
	}

	// Score Management
	scores := r.Group("/scores")
	{
		scores.POST("/recalculate", h.recalculateScore)
	}

	// Cluster Management
	clusters := r.Group("/clusters")
	{
		clusters.GET("/status", h.getClusterStatus)
		clusters.GET("/:clusterName/nodes/:nodeName", h.getNodeDetails)
		clusters.POST("/:clusterName/nodes/:nodeName/pause", h.pauseNode)
		clusters.POST("/:clusterName/nodes/:nodeName/resume", h.resumeNode)
	}

	// Container Management
	containers := r.Group("/containers")
	{
		containers.GET("", h.getAllContainers)
		containers.GET("/:id", h.getContainer)
	}

	return r
}
