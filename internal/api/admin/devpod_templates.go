package admin

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/devpods"
	"github.com/ZJUSCT/CSOJ/internal/kubeutil"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)

var templateIDRE = regexp.MustCompile(`^[a-z0-9-]{1,6}$`)

func (h *Handler) listDevPodTemplates(c *gin.Context) {
	rows, err := database.ListDevPodTemplates(h.db)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, rows, "Templates retrieved")
}

func (h *Handler) createDevPodTemplate(c *gin.Context) {
	var tpl models.DevPodTemplate
	if err := c.ShouldBindJSON(&tpl); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if !templateIDRE.MatchString(tpl.ID) {
		util.Error(c, http.StatusBadRequest, "id must match [a-z0-9-]{1,6}")
		return
	}
	if tpl.Name == "" || tpl.ClusterName == "" || tpl.Image == "" {
		util.Error(c, http.StatusBadRequest, "name, cluster_name, image are required")
		return
	}
	if tpl.Cores <= 0 || tpl.Memory <= 0 {
		util.Error(c, http.StatusBadRequest, "cores and memory must be positive")
		return
	}
	if err := normalizeDevPodTemplateGPU(&tpl); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	tpl.AllowedTags = devpods.NormalizeAllowedTags(tpl.AllowedTags)
	if tpl.DefaultPerUser <= 0 {
		tpl.DefaultPerUser = 1
	}
	if tpl.DefaultGlobal <= 0 {
		tpl.DefaultGlobal = 5
	}
	// cluster must exist
	if _, err := database.GetCluster(h.db, tpl.ClusterName); err != nil {
		util.Error(c, http.StatusBadRequest, "referenced cluster does not exist")
		return
	}
	if err := database.CreateDevPodTemplate(h.db, &tpl); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, tpl, "Template created")
}

func (h *Handler) updateDevPodTemplate(c *gin.Context) {
	id := c.Param("id")
	var tpl models.DevPodTemplate
	if err := c.ShouldBindJSON(&tpl); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if id != tpl.ID {
		util.Error(c, http.StatusBadRequest, "id in path does not match body")
		return
	}
	if tpl.Name == "" || tpl.ClusterName == "" || tpl.Image == "" {
		util.Error(c, http.StatusBadRequest, "name, cluster_name, image are required")
		return
	}
	if tpl.Cores <= 0 || tpl.Memory <= 0 {
		util.Error(c, http.StatusBadRequest, "cores and memory must be positive")
		return
	}
	if err := normalizeDevPodTemplateGPU(&tpl); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	tpl.AllowedTags = devpods.NormalizeAllowedTags(tpl.AllowedTags)
	if tpl.DefaultPerUser <= 0 {
		tpl.DefaultPerUser = 1
	}
	if tpl.DefaultGlobal <= 0 {
		tpl.DefaultGlobal = 5
	}
	if _, err := database.GetCluster(h.db, tpl.ClusterName); err != nil {
		util.Error(c, http.StatusBadRequest, "referenced cluster does not exist")
		return
	}
	existing, err := database.GetDevPodTemplate(h.db, id)
	if err != nil {
		util.Error(c, http.StatusNotFound, "template not found")
		return
	}
	tpl.CreatedAt = existing.CreatedAt
	tpl.UpdatedAt = existing.UpdatedAt
	if err := database.UpdateDevPodTemplate(h.db, &tpl); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, tpl, "Template updated")
}

func (h *Handler) deleteDevPodTemplate(c *gin.Context) {
	id := c.Param("id")
	if err := database.DeleteDevPodTemplate(h.db, id); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, nil, "Template deleted")
}

func normalizeDevPodTemplateGPU(tpl *models.DevPodTemplate) error {
	if tpl.GPUCount < 0 {
		return fmt.Errorf("gpu_count must be non-negative")
	}
	if tpl.GPUCount == 0 {
		return nil
	}
	name, err := kubeutil.NormalizeGPUResourceName(tpl.GPUResource)
	if err != nil {
		return err
	}
	tpl.GPUResource = name
	return nil
}
