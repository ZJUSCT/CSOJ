package admin

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ZJUSCT/CSOJ/internal/devpods"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)

// listSettings returns all settings rows + boot facts for display.
func (h *Handler) listSettings(c *gin.Context) {
	rows, err := h.settings.ListAll()
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, gin.H{
		"settings": rows,
		"boot": gin.H{
			"listen":             h.cfg.Listen,
			"storage":            h.cfg.Storage,
			"jwt_secret_present": h.cfg.Auth.JWT.Secret != "",
		},
	}, "Settings retrieved")
}

// updateSetting writes a settings value.
func (h *Handler) updateSetting(c *gin.Context) {
	key := c.Param("key")
	var body struct {
		Value interface{} `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if key == "devpods.gateway" {
		raw, err := json.Marshal(body.Value)
		if err != nil {
			util.Error(c, http.StatusBadRequest, "invalid devpod gateway setting")
			return
		}
		var gateway devpods.Gateway
		if err := json.Unmarshal(raw, &gateway); err != nil {
			util.Error(c, http.StatusBadRequest, "devpod gateway must contain host and numeric port")
			return
		}
		gateway.Host = strings.TrimSpace(gateway.Host)
		gateway.HostnameSuffix = strings.TrimSpace(gateway.HostnameSuffix)
		gateway.AuditNamespace = strings.TrimSpace(gateway.AuditNamespace)
		if err := gateway.Validate(); err != nil {
			util.Error(c, http.StatusBadRequest, err)
			return
		}
		body.Value = gateway
	}
	if key == "devpods.max_per_user" {
		raw, err := json.Marshal(body.Value)
		if err != nil {
			util.Error(c, http.StatusBadRequest, "invalid devpod user limit")
			return
		}
		var limit int
		if err := json.Unmarshal(raw, &limit); err != nil || limit < 0 {
			util.Error(c, http.StatusBadRequest, "devpod max per user must be a non-negative integer")
			return
		}
		body.Value = limit
	}
	if err := h.settings.Set(key, body.Value); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	restart := key == "logger"
	util.Success(c, gin.H{"restart_required": restart}, "Setting updated")
}
