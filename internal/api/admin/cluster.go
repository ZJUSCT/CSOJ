package admin

import (
	"net/http"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)

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
