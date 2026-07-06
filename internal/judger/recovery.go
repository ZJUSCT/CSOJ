package judger

import (
	"context"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/pubsub"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// RecoverAndCleanup claims each cluster (HA gate), then deletes all judger
// pods/MPIJobs and marks Running submissions Failed.
func RecoverAndCleanup(db *gorm.DB, cfg *config.Config, instanceID string) error {
	// HA gate: claim every configured cluster before touching K8s.
	for i := range cfg.Cluster {
		cc := cfg.Cluster[i]
		ttl := cc.HeartbeatTTL.Std()
		if ttl <= 0 {
			ttl = 30 * time.Second
		}
		claimed, err := ClaimCluster(db, cc.Name, instanceID, ttl)
		if err != nil {
			return err
		}
		if !claimed {
			zap.S().Fatalf("cluster %s is owned by another live judger; aborting", cc.Name)
		}
	}

	// Build K8s managers and delete all judger-owned resources.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for i := range cfg.Cluster {
		cc := cfg.Cluster[i]
		km, err := buildKubeManagerForCluster(cc)
		if err != nil {
			zap.S().Errorf("failed to build K8s client for cluster %s: %v", cc.Name, err)
			continue
		}
		if err := km.DeleteAllJudgerPods(ctx); err != nil {
			zap.S().Warnf("cluster %s: failed to delete judger pods: %v", cc.Name, err)
		}
		if err := km.DeleteAllJudgerMPIJobs(ctx); err != nil {
			zap.S().Warnf("cluster %s: failed to delete judger MPIJobs: %v", cc.Name, err)
		}
	}

	// Mark all Running submissions + their containers Failed.
	var running []models.Submission
	if err := db.Preload("Containers").Where("status = ?", models.StatusRunning).Find(&running).Error; err != nil {
		return err
	}
	tx := db.Begin()
	for i := range running {
		running[i].Status = models.StatusFailed
		running[i].Info = models.JSONMap{"error": "System interrupted during execution"}
		if err := tx.Save(&running[i]).Error; err != nil {
			tx.Rollback()
			return err
		}
		pubsub.GetBroker().CloseTopic(running[i].ID)
		for j := range running[i].Containers {
			running[i].Containers[j].Status = models.StatusFailed
			if err := tx.Save(&running[i].Containers[j]).Error; err != nil {
				tx.Rollback()
				return err
			}
		}
	}
	return tx.Commit().Error
}

// buildKubeManagerForCluster builds a KubeManager from a config.Cluster using
// the kubeconfig + context (same construction as NewScheduler).
func buildKubeManagerForCluster(cc config.Cluster) (*KubeManager, error) {
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: cc.Kubeconfig},
		&clientcmd.ConfigOverrides{CurrentContext: cc.Context},
	)
	restCfg, err := loader.ClientConfig()
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}
	dyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}
	return NewKubeManager(cs, dyn, cc.Namespace), nil
}
