package judger

import (
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ClaimCluster waits up to ttl for any prior heartbeat to expire, then claims
// the cluster for this instance. Returns (true, nil) on claim, (false, nil) if
// another live instance holds it.
func ClaimCluster(db *gorm.DB, clusterName, instanceID string, ttl time.Duration) (bool, error) {
	hb, err := database.GetHeartbeat(db, clusterName)
	if err != nil {
		return false, err
	}
	if hb != nil && hb.InstanceID != instanceID && time.Since(hb.LastBeatAt) < ttl {
		return false, nil
	}
	// Claim (or re-claim).
	return true, database.UpsertHeartbeat(db, &models.Heartbeat{
		ClusterName: clusterName, InstanceID: instanceID, LastBeatAt: time.Now(),
	})
}

// StartHeartbeat writes a heartbeat every ttl/2 until ctx is done.
func StartHeartbeat(db *gorm.DB, clusterName, instanceID string, ttl time.Duration, stop <-chan struct{}) {
	interval := ttl / 2
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if err := database.UpsertHeartbeat(db, &models.Heartbeat{
				ClusterName: clusterName, InstanceID: instanceID, LastBeatAt: time.Now(),
			}); err != nil {
				zap.S().Errorf("heartbeat write failed for %s: %v", clusterName, err)
			}
		}
	}
}
