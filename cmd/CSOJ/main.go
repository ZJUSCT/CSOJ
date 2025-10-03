package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ZJUSCT/CSOJ/internal/api"
	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/judger"

	"go.uber.org/zap"
)

func main() {
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

	// recover interrupted submissions
	if err := database.RecoverInterrupted(db); err != nil {
		zap.S().Errorf("failed to recover interrupted submissions: %v", err)
	} else {
		zap.S().Info("successfully recovered interrupted submissions")
	}

	// contests and problems
	contests, problems, err := judger.LoadAllContestsAndProblems(cfg.Contest)
	if err != nil {
		zap.S().Fatalf("failed to load contests and problems: %v", err)
	}
	zap.S().Infof("loaded %d contests and %d problems", len(contests), len(problems))

	// Helper map to find the parent contest of a problem
	problemToContestMap := make(map[string]*judger.Contest)
	for _, contest := range contests {
		for _, problemID := range contest.ProblemIDs {
			problemToContestMap[problemID] = contest
		}
	}

	// judger scheduler
	scheduler := judger.NewScheduler(cfg, db)

	// Requeue pending submissions from the last run
	if err := judger.RequeuePendingSubmissions(db, scheduler, problems); err != nil {
		zap.S().Fatalf("failed to requeue pending submissions: %v", err)
	}

	go scheduler.Run()
	zap.S().Info("judger scheduler started")

	// API routers
	userEngine := api.NewUserRouter(cfg, db, scheduler, contests, problems, problemToContestMap)
	adminEngine := api.NewAdminRouter(cfg, db, scheduler, contests, problems, problemToContestMap)

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
