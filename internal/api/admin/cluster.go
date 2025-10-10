package admin

import (
	"fmt"
	"net/http"

	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
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
