package judger

import (
	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/pubsub"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Dispatcher struct {
	cfg       *config.Config
	db        *gorm.DB
	scheduler *Scheduler
	appState  *AppState
}

func NewDispatcher(cfg *config.Config, db *gorm.DB, scheduler *Scheduler, appState *AppState) *Dispatcher {
	return &Dispatcher{cfg: cfg, db: db, scheduler: scheduler, appState: appState}
}

// Dispatch is a stub pending the Phase D rewrite. It fails the submission
// immediately so queued submissions don't hang.
func (d *Dispatcher) Dispatch(sub *models.Submission, prob *Problem, cluster *ClusterState, pool *PoolState) {
	zap.S().Warnf("dispatcher stub: failing submission %s (Phase D not yet implemented)", sub.ID)
	pubsub.GetBroker().Publish(sub.ID, pubsub.FormatMessage("error", "dispatcher not yet implemented (Phase D)"))
	sub.Status = models.StatusFailed
	sub.Info = models.JSONMap{"error": "dispatcher not yet implemented (Phase D)"}
	database.UpdateSubmission(d.db, sub)
	d.scheduler.ReleaseSlot(cluster.Name)
	pubsub.GetBroker().CloseTopic(sub.ID)
}
