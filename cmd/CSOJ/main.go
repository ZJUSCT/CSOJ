package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/api"
	"github.com/ZJUSCT/CSOJ/internal/api/admin"
	"github.com/ZJUSCT/CSOJ/internal/api/user"
	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/embedui"
	"github.com/ZJUSCT/CSOJ/internal/judger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"go.uber.org/zap"
)

var Version = "dev-build"

func main() {

	fmt.Fprintf(os.Stderr, "ZJUSCT CSOJ %s - Fully Containerized Secure Online Judgement\n\n", Version)

	// config (boot facts only: listen, storage, auth.jwt.secret + expire_hours fallback)
	var configPath string
	flag.StringVar(&configPath, "c", "configs/config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// database
	db, err := database.Init(cfg.Storage.Database)
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}

	// SettingsStore (runtime settings from the DB)
	settings := config.NewSettingsStore(db)

	// Logger (level/file from settings; restart-required to change)
	var loggerCfg config.LoggerConfig
	_ = settings.Get("logger", &loggerCfg)
	if loggerCfg.Level == "" {
		loggerCfg.Level = "info"
	}
	var zapCfg zap.Config
	if loggerCfg.Level == "debug" {
		zapCfg = zap.NewDevelopmentConfig()
	} else {
		zapCfg = zap.NewProductionConfig()
	}
	if loggerCfg.File != "" {
		zapCfg.OutputPaths = []string{"stdout", loggerCfg.File}
		zapCfg.ErrorOutputPaths = []string{"stderr", loggerCfg.File}
	} else {
		zapCfg.OutputPaths = []string{"stdout"}
		zapCfg.ErrorOutputPaths = []string{"stderr"}
	}
	logger, err := zapCfg.Build()
	if err != nil {
		log.Fatalf("can't initialize zap logger: %v", err)
	}
	defer logger.Sync()
	zap.ReplaceGlobals(logger)
	zap.S().Info("database initialized successfully")

	// recovery and cleanup (HA-gated per cluster; clusters from DB)
	instanceID := uuid.NewString()
	if err := judger.RecoverAndCleanup(db, instanceID); err != nil {
		zap.S().Errorf("failed to recover and cleanup: %v", err)
	} else {
		zap.S().Info("successfully recovered and cleaned up interrupted tasks")
	}

	// AppState holds the shared, reloadable state
	appState := &judger.AppState{
		RWMutex:             sync.RWMutex{},
		Contests:            make(map[string]*judger.Contest),
		Problems:            make(map[string]*judger.Problem),
		ProblemToContestMap: make(map[string]*judger.Contest),
	}

	// contests and problems (loaded from the DB)
	contests, problems, problemToContestMap, err := judger.LoadFromDB(db)
	if err != nil {
		zap.S().Fatalf("failed to load contests and problems: %v", err)
	}
	appState.Contests = contests
	appState.Problems = problems
	appState.ProblemToContestMap = problemToContestMap
	zap.S().Infof("loaded %d contests and %d problems", len(contests), len(problems))

	// judger scheduler (clusters from DB; cfg threaded for the dispatcher)
	scheduler := judger.NewScheduler(db, settings, cfg, appState)

	// Requeue pending submissions from the last run
	if err := judger.RequeuePendingSubmissions(db, scheduler, appState); err != nil {
		zap.S().Fatalf("failed to requeue pending submissions: %v", err)
	}

	// Start HA heartbeats (one goroutine per cluster row in the DB).
	hbStop := make(chan struct{})
	dbClusters, err := database.GetAllClusters(db)
	if err != nil {
		zap.S().Fatalf("failed to load clusters for heartbeat: %v", err)
	}
	for _, cc := range dbClusters {
		cc := cc
		ttl := time.Duration(cc.HeartbeatTTL) * time.Second
		if ttl <= 0 {
			ttl = 30 * time.Second
		}
		go judger.StartHeartbeat(db, cc.Name, instanceID, ttl, hbStop)
	}

	go scheduler.Run()
	zap.S().Info("judger scheduler started")

	// Single API engine
	r := gin.Default()
	r.Use(api.CORSMiddleware(settings))
	user.RegisterRoutes(r, cfg, settings, db, scheduler, appState)
	admin.RegisterRoutes(r, cfg, settings, db, scheduler, appState)
	embedui.RegisterUIHandlers(r, "user")

	// start server
	go func() {
		zap.S().Infof("starting server at %s", cfg.Listen)
		if err := r.Run(cfg.Listen); err != nil {
			zap.S().Fatalf("failed to start server: %v", err)
		}
	}()

	// graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	close(hbStop)
	zap.S().Info("shutting down server...")
}
