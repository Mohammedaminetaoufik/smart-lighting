package main

import (
"github.com/gin-gonic/gin"
"database/sql"
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
