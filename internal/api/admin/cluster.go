package admin

import (
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)

func (h *Handler) getClusterStatus(c *gin.Context) {
	status := h.scheduler.GetClusterStates()
	util.Success(c, status, "Cluster status retrieved")
}
