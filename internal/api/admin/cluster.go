package admin

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)

type clusterMutationResponse struct {
	models.Cluster
	Warnings []string `json:"warnings"`
}

func (h *Handler) getClusterStatus(c *gin.Context) {
	states := h.scheduler.GetClusterStates()
	queueLengths := h.scheduler.GetQueueLengths()
	util.Success(c, gin.H{"resource_status": states, "queue_lengths": queueLengths}, "Cluster status retrieved")
}

func (h *Handler) listPools(c *gin.Context) {
	cluster := c.Param("cluster")
	pools, err := database.GetClusterPools(h.db, cluster)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, pools, "Pools retrieved")
}

func (h *Handler) createPool(c *gin.Context) {
	cluster := c.Param("cluster")
	var pool models.ClusterNodePool
	if err := c.ShouldBindJSON(&pool); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	pool.ClusterName = cluster
	if err := database.UpsertClusterPool(h.db, &pool); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, pool, "Pool created")
}

func (h *Handler) updatePool(c *gin.Context) {
	cluster := c.Param("cluster")
	poolName := c.Param("pool")
	var req struct {
		CPU          int            `json:"cpu"`
		Memory       int64          `json:"memory"`
		NodeSelector models.JSONMap `json:"node_selector"`
		IsPaused     *bool          `json:"is_paused"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	pool := models.ClusterNodePool{ClusterName: cluster, PoolName: poolName, CPU: req.CPU, Memory: req.Memory, NodeSelector: req.NodeSelector}
	if req.IsPaused != nil {
		pool.IsPaused = *req.IsPaused
	}
	if err := database.UpsertClusterPool(h.db, &pool); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	// Refresh in-memory scheduler state.
	_ = h.scheduler.UpdatePoolResources(cluster, poolName, req.CPU, req.Memory)
	if req.IsPaused != nil {
		if *req.IsPaused {
			_ = h.scheduler.PausePool(cluster, poolName)
		} else {
			_ = h.scheduler.ResumePool(cluster, poolName)
		}
	}
	util.Success(c, pool, "Pool updated")
}

func (h *Handler) deletePool(c *gin.Context) {
	cluster := c.Param("cluster")
	pool := c.Param("pool")
	if err := database.DeleteClusterPool(h.db, cluster, pool); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, nil, "Pool deleted")
}

func (h *Handler) setConcurrency(c *gin.Context) {
	cluster := c.Param("cluster")
	var req struct {
		Concurrency int `json:"concurrency"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if err := h.scheduler.SetConcurrency(cluster, req.Concurrency); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	util.Success(c, gin.H{"cluster": cluster, "concurrency": req.Concurrency}, "Concurrency updated (restart to resize)")
}

// --- Cluster-row CRUD (DB-managed clusters; distinct from pool management) ---

// listClusters returns all cluster rows. Kubeconfig text is omitted from the
// response because it may contain secrets.
func (h *Handler) listClusters(c *gin.Context) {
	rows, err := database.GetAllClusters(h.db)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	type clusterSummary struct {
		Name         string `json:"name"`
		Context      string `json:"context"`
		Namespace    string `json:"namespace"`
		Concurrency  int    `json:"concurrency"`
		HeartbeatTTL int    `json:"heartbeat_ttl"`
		QueueMode    string `json:"queue_mode"`
	}
	out := make([]clusterSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, clusterSummary{
			Name: r.Name, Context: r.Context, Namespace: r.Namespace,
			Concurrency: r.Concurrency, HeartbeatTTL: r.HeartbeatTTL,
			QueueMode: normalizedQueueMode(r.QueueMode),
		})
	}
	util.Success(c, out, "Clusters retrieved")
}

// createCluster creates a new cluster row (or upserts if the name exists).
func (h *Handler) createCluster(c *gin.Context) {
	var cl models.Cluster
	if err := c.ShouldBindJSON(&cl); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	queueMode, err := validateQueueMode(cl.QueueMode)
	if err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	cl.QueueMode = queueMode
	if err := database.UpsertCluster(h.db, &cl); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	warnings, err := h.scheduler.ReloadClusters()
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("cluster saved but scheduler reload failed: %w", err))
		return
	}
	util.Success(c, clusterMutationResponse{Cluster: cl, Warnings: warnings}, "Cluster created")
}

// updateCluster updates an existing cluster row by name (including kubeconfig).
func (h *Handler) updateCluster(c *gin.Context) {
	name := c.Param("cluster")
	var cl models.Cluster
	if err := c.ShouldBindJSON(&cl); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if name != cl.Name {
		util.Error(c, http.StatusBadRequest, "cluster name in path does not match body")
		return
	}
	queueMode, err := validateQueueMode(cl.QueueMode)
	if err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	cl.QueueMode = queueMode
	existing, err := database.GetCluster(h.db, name)
	if err != nil {
		util.Error(c, http.StatusNotFound, "cluster not found")
		return
	}
	// Cluster summaries deliberately omit kubeconfig secrets. Preserve the
	// stored kubeconfig when the edit form submits an empty value.
	if strings.TrimSpace(cl.Kubeconfig) == "" {
		cl.Kubeconfig = existing.Kubeconfig
	}
	cl.CreatedAt = existing.CreatedAt
	if err := database.UpsertCluster(h.db, &cl); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	warnings, err := h.scheduler.ReloadClusters()
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("cluster saved but scheduler reload failed: %w", err))
		return
	}
	util.Success(c, clusterMutationResponse{Cluster: cl, Warnings: warnings}, "Cluster updated")
}

func normalizedQueueMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return "channel"
	}
	return mode
}

func validateQueueMode(mode string) (string, error) {
	mode = normalizedQueueMode(mode)
	if mode != "channel" && mode != "kueue" {
		return "", fmt.Errorf("queue_mode must be 'channel' or 'kueue'")
	}
	return mode, nil
}

// deleteCluster deletes a cluster row by name.
func (h *Handler) deleteCluster(c *gin.Context) {
	name := c.Param("cluster")
	if err := database.DeleteCluster(h.db, name); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, nil, "Cluster deleted")
}

// reloadClusters rebuilds the scheduler's in-memory cluster clientsets from the
// DB without restarting the server.
func (h *Handler) reloadClusters(c *gin.Context) {
	warnings, err := h.scheduler.ReloadClusters()
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, gin.H{"warnings": warnings}, "Clusters reloaded")
}
