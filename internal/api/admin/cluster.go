package admin

import (
	"fmt"
	"net/http"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (h *Handler) getClusterStatus(c *gin.Context) {
	// This structure combines resource status and queue status
	type ClusterStatusResponse struct {
		ResourceStatus interface{}    `json:"resource_status"`
		QueueLengths   map[string]int `json:"queue_lengths"`
	}

	status := h.scheduler.GetClusterStates()
	queueLengths := h.scheduler.GetQueueLengths()

	response := ClusterStatusResponse{
		ResourceStatus: status,
		QueueLengths:   queueLengths,
	}

	util.Success(c, response, "Cluster status retrieved")
}

func (h *Handler) getNodeDetails(c *gin.Context) {
	clusterName := c.Param("clusterName")
	nodeName := c.Param("nodeName")

	details, err := h.scheduler.GetNodeDetails(clusterName, nodeName)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}
	util.Success(c, details, "Node details retrieved successfully")
}

func (h *Handler) pauseNode(c *gin.Context) {
	clusterName := c.Param("clusterName")
	nodeName := c.Param("nodeName")

	if err := h.scheduler.PauseNode(clusterName, nodeName); err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}
	util.Success(c, nil, fmt.Sprintf("Node '%s/%s' paused successfully", clusterName, nodeName))
}

func (h *Handler) resumeNode(c *gin.Context) {
	clusterName := c.Param("clusterName")
	nodeName := c.Param("nodeName")

	if err := h.scheduler.ResumeNode(clusterName, nodeName); err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}
	util.Success(c, nil, fmt.Sprintf("Node '%s/%s' resumed successfully", clusterName, nodeName))
}

func (h *Handler) updateNode(c *gin.Context) {
	clusterName := c.Param("clusterName")
	nodeName := c.Param("nodeName")
	var req struct {
		CPU    int   `json:"cpu" binding:"required"`
		Memory int64 `json:"memory" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if req.CPU <= 0 || req.Memory <= 0 {
		util.Error(c, http.StatusBadRequest, "cpu and memory must be positive")
		return
	}

	// Verify the node exists in the scheduler (its docker connection is in config.yaml).
	if _, err := h.scheduler.GetNodeDetails(clusterName, nodeName); err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}

	node := models.ClusterNode{
		ClusterName: clusterName,
		NodeName:    nodeName,
		CPU:         req.CPU,
		Memory:      req.Memory,
	}
	if err := database.UpsertClusterNode(h.db, &node); err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update node: %w", err))
		return
	}
	if err := h.scheduler.UpdateNodeResources(clusterName, nodeName, req.CPU, req.Memory); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	zap.S().Infof("admin updated node %s/%s resources: cpu=%d memory=%d", clusterName, nodeName, req.CPU, req.Memory)
	util.Success(c, node, "Node resources updated")
}
