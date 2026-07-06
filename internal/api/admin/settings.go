package admin

import (
	"net/http"

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
	if err := h.settings.Set(key, body.Value); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	restart := key == "logger"
	util.Success(c, gin.H{"restart_required": restart}, "Setting updated")
}
