package admin

import (
	"net/http"
	"strconv"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)

func (h *Handler) listLinks(c *gin.Context) {
	links, err := database.GetAllLinks(h.db)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, links, "Links retrieved")
}

func (h *Handler) createLink(c *gin.Context) {
	var link models.Link
	if err := c.ShouldBindJSON(&link); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if link.Position == 0 {
		existing, _ := database.GetAllLinks(h.db)
		link.Position = len(existing)
	}
	if err := database.CreateLink(h.db, &link); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, link, "Link created")
}

func (h *Handler) updateLink(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.Error(c, http.StatusBadRequest, "invalid link id")
		return
	}
	var link models.Link
	if err := c.ShouldBindJSON(&link); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	link.ID = uint(id)
	if err := database.UpdateLink(h.db, &link); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, link, "Link updated")
}

func (h *Handler) deleteLink(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.Error(c, http.StatusBadRequest, "invalid link id")
		return
	}
	if err := database.DeleteLink(h.db, uint(id)); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, nil, "Link deleted")
}
