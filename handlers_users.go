package main

import (
"github.com/gin-gonic/gin"
"database/sql"
)

func handleGetUsers(db *sql.DB) gin.HandlerFunc {
return func(c *gin.Context) {
users, err := listUsers(c.Request.Context(), db)
if err != nil {
c.JSON(500, gin.H{"error": err.Error()})
return
}
c.JSON(200, users)
}
}

func handleCreateUser(db *sql.DB) gin.HandlerFunc {
return func(c *gin.Context) {
var u User
if err := c.ShouldBindJSON(&u); err != nil {
c.JSON(400, gin.H{"error": err.Error()})
return
}
id, err := insertUser(c.Request.Context(), db, u)
if err != nil {
c.JSON(500, gin.H{"error": err.Error()})
return
}
u.ID = id
c.JSON(200, u)
}
}
