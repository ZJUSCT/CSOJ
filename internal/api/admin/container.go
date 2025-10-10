package admin

import (
	"net/http"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)

func (h *Handler) getAllContainers(c *gin.Context) {
	filters := make(map[string]string)
	if submissionID := c.Query("submission_id"); submissionID != "" {
		filters["submission_id"] = submissionID
	}
	if userID := c.Query("user_id"); userID != "" {
		filters["user_id"] = userID
	}
	if status := c.Query("status"); status != "" {
		filters["status"] = status
	}

	containers, err := database.GetAllContainers(h.db, filters)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, containers, "Containers retrieved successfully")
}

func (h *Handler) getContainer(c *gin.Context) {
	containerID := c.Param("id")
	container, err := database.GetContainer(h.db, containerID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}
	util.Success(c, container, "Container retrieved successfully")
}
