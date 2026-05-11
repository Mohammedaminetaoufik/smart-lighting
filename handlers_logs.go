package main

import (
	"database/sql"
	"github.com/gin-gonic/gin"
)

func handleGetLogs(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		logs, err := listAccessLogs(c.Request.Context(), db)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, logs)
	}
}
