package admin

import (
	"math"
	"net/http"
	"strconv"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)

func (h *Handler) getAllContainers(c *gin.Context) {
	// Pagination parameters
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "20")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 20
	}
	if limit > 100 { // Add a reasonable upper bound for limit
		limit = 100
	}

	offset := (page - 1) * limit

	// Filters
	filters := make(map[string]string)
	if submissionID := c.Query("submission_id"); submissionID != "" {
		filters["submission_id"] = submissionID
	}
	if status := c.Query("status"); status != "" {
		filters["status"] = status
	}
	if userQuery := c.Query("user_query"); userQuery != "" {
		filters["user_query"] = userQuery
	}

	containers, totalItems, err := database.GetAllContainers(h.db, filters, limit, offset)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	totalPages := int(math.Ceil(float64(totalItems) / float64(limit)))

	response := gin.H{
		"items":        containers,
		"total_items":  totalItems,
		"total_pages":  totalPages,
		"current_page": page,
		"per_page":     limit,
	}

	util.Success(c, response, "Containers retrieved successfully")
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
