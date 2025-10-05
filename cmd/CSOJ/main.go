package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/ZJUSCT/CSOJ/internal/api/admin"
	"github.com/ZJUSCT/CSOJ/internal/api/user"
	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/judger"

	"go.uber.org/zap"
)

var Version = "dev-build"

func main() {

	fmt.Fprintf(os.Stderr, "ZJUSCT CSOJ %s - Fully Containerized Secure Online Judgement\n\n", Version)

	// config
	var configPath string
	flag.StringVar(&configPath, "c", "configs/config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// logger
	var logger *zap.Logger
	if cfg.Logger.Level == "debug" {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}
	if err != nil {
		log.Fatalf("can't initialize zap logger: %v", err)
	}
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	// database
	db, err := database.Init(cfg.Storage.Database)
	if err != nil {
		zap.S().Fatalf("failed to initialize database: %v", err)
	}
	zap.S().Info("database initialized successfully")

	// recovery and cleanup
	if err := judger.RecoverAndCleanup(db, cfg); err != nil {
		zap.S().Errorf("failed to recover and cleanup interrupted tasks: %v", err)
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

	// contests and problems
	contests, problems, err := judger.LoadAllContestsAndProblems(cfg.Contest)
	if err != nil {
		zap.S().Fatalf("failed to load contests and problems: %v", err)
	}
	appState.Contests = contests
	appState.Problems = problems
	zap.S().Infof("loaded %d contests and %d problems", len(contests), len(problems))

	// Helper map to find the parent contest of a problem
	problemToContestMap := make(map[string]*judger.Contest)
	for _, contest := range contests {
		for _, problemID := range contest.ProblemIDs {
			problemToContestMap[problemID] = contest
		}
	}
	appState.ProblemToContestMap = problemToContestMap

	// judger scheduler
	scheduler := judger.NewScheduler(cfg, db, appState)

	// Requeue pending submissions from the last run
	if err := judger.RequeuePendingSubmissions(db, scheduler, appState); err != nil {
		zap.S().Fatalf("failed to requeue pending submissions: %v", err)
	}

	go scheduler.Run()
	zap.S().Info("judger scheduler started")

	// API routers
	userEngine := user.NewUserRouter(cfg, db, scheduler, appState)    // <-- 修改
	adminEngine := admin.NewAdminRouter(cfg, db, scheduler, appState) // <-- 修改

	// start servers
	go func() {
		zap.S().Infof("starting user server at %s", cfg.Listen)
		if err := userEngine.Run(cfg.Listen); err != nil {
			zap.S().Fatalf("failed to start user server: %v", err)
		}
	}()

	if cfg.Admin.Enabled {
		go func() {
			zap.S().Infof("starting admin server at %s", cfg.Admin.Listen)
			if err := adminEngine.Run(cfg.Admin.Listen); err != nil {
				zap.S().Fatalf("failed to start admin server: %v", err)
			}
		}()
	}

	// graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	zap.S().Info("shutting down server...")
}
