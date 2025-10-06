package admin

import (
	"net/http"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) getAllUsers(c *gin.Context) {
	users, err := database.GetAllUsers(h.db)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, users, "Users retrieved successfully")
}

func (h *Handler) createUser(c *gin.Context) {
	var user models.User
	if err := c.ShouldBindJSON(&user); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	user.ID = uuid.NewString()
	if err := database.CreateUser(h.db, &user); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, user, "User created successfully")
}

func (h *Handler) deleteUser(c *gin.Context) {
	userID := c.Param("id")
	if err := database.DeleteUser(h.db, userID); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, nil, "User deleted successfully")
}