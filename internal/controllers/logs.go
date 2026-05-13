package controllers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/repository"
)

// HandleGetLogs handles GET /api/logs.
func HandleGetLogs(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		logs, err := repository.ListAccessLogs(c.Request.Context(), db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		RespondJSON(c, http.StatusOK, logs)
	}
}
