package judger

import (
	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// RecoverAndCleanup handles the recovery process on application startup.
// It finds submissions and containers that were in a 'Running' state
// and cleans up their associated Docker containers before marking them
// as 'Failed' in the database.
func RecoverAndCleanup(db *gorm.DB, cfg *config.Config) error {
	zap.S().Info("starting recovery process for interrupted submissions...")

	// 查找所有在运行时被中断的提交，并预加载它们关联的所有容器
	var interruptedSubs []models.Submission
	if err := db.Preload("Containers").Where("status = ?", models.StatusRunning).Find(&interruptedSubs).Error; err != nil {
		return err
	}

	if len(interruptedSubs) == 0 {
		zap.S().Info("no interrupted submissions found to recover")
		return nil
	}
	zap.S().Infof("found %d interrupted submissions to process", len(interruptedSubs))

	// 创建一个快速查找表，用于根据集群和节点名找到对应的 Docker Host 配置
	nodeConfigMap := make(map[string]map[string]*config.Node)
	for i := range cfg.Cluster {
		cluster := cfg.Cluster[i]
		nodeConfigMap[cluster.Name] = make(map[string]*config.Node)
		for j := range cluster.Nodes {
			node := cluster.Nodes[j]
			nodeConfigMap[cluster.Name][node.Name] = &node
		}
	}

	// 按 Docker Host 对所有需要清理的容器进行分组
	containersByHost := make(map[string][]*models.Container)
	var submissionIDs []string

	for _, sub := range interruptedSubs {
		submissionIDs = append(submissionIDs, sub.ID)
		if sub.Cluster == "" || sub.Node == "" {
			zap.S().Warnf("submission %s has no cluster/node assigned, cannot clean up its containers", sub.ID)
			continue
		}

		// 查找该提交所在节点的 Docker Host
		clusterNodes, ok := nodeConfigMap[sub.Cluster]
		if !ok {
			zap.S().Warnf("cluster '%s' for submission %s not found in config, cannot clean up containers", sub.Cluster, sub.ID)
			continue
		}
		node, ok := clusterNodes[sub.Node]
		if !ok {
			zap.S().Warnf("node '%s' for submission %s not found in config, cannot clean up containers", sub.Node, sub.ID)
			continue
		}
		dockerHost := node.Docker

		// 将该提交下所有拥有 DockerID 的容器加入对应 Host 的清理列表
		for i := range sub.Containers {
			container := sub.Containers[i]
			if container.DockerID != "" {
				containersByHost[dockerHost] = append(containersByHost[dockerHost], &container)
			}
		}
	}

	// 执行清理操作
	for host, containers := range containersByHost {
		zap.S().Infof("connecting to Docker host %s to clean up %d containers", host, len(containers))
		docker, err := NewDockerManager(host)
		if err != nil {
			zap.S().Errorf("failed to create Docker manager for host %s: %v. Skipping cleanup for this host.", host, err)
			continue
		}
		for _, container := range containers {
			zap.S().Infof("cleaning up orphaned container %s (DockerID: %s) on host %s", container.ID, container.DockerID, host)
			docker.CleanupContainer(container.DockerID)
		}
	}

	// 清理完成后，在一个事务中更新数据库记录
	zap.S().Info("updating database status for interrupted submissions and containers")
	return db.Transaction(func(tx *gorm.DB) error {
		// 将被中断的提交标记为失败
		if err := tx.Model(&models.Submission{}).
			Where("id IN ?", submissionIDs).
			Updates(map[string]interface{}{
				"status": models.StatusFailed,
				"info":   models.JSONMap{"error": "System interrupted during execution"},
			}).Error; err != nil {
			return err
		}

		// 将关联的、正在运行的容器标记为失败
		if err := tx.Model(&models.Container{}).
			Where("submission_id IN ?", submissionIDs).
			Where("status = ?", models.StatusRunning).
			Update("status", models.StatusFailed).Error; err != nil {
			return err
		}
		return nil
	})
}
