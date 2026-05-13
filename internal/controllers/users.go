package controllers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/models"
	"map-interactif/internal/repository"
)

// HandleGetUsers handles GET /api/users.
func HandleGetUsers(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		users, err := repository.ListUsers(c.Request.Context(), db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		RespondJSON(c, http.StatusOK, users)
	}
}

// HandleCreateUser handles POST /api/users.
func HandleCreateUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var u models.User
		if err := c.ShouldBindJSON(&u); err != nil {
			RespondError(c, http.StatusBadRequest, err.Error())
			return
		}
		id, err := repository.InsertUser(c.Request.Context(), db, u)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		u.ID = id
		RespondJSON(c, http.StatusOK, u)
	}
}
