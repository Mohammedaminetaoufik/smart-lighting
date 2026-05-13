package controllers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// RespondJSON sends a JSON response with the given status code.
func RespondJSON(c *gin.Context, status int, data interface{}) {
	c.JSON(status, data)
}

// RespondError sends a JSON error response.
func RespondError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

// ParseIDParam parses an integer ID from a Gin URL parameter.
func ParseIDParam(c *gin.Context, name string) (int, error) {
	id, err := strconv.Atoi(c.Param(name))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("identifiant invalide")
	}
	return id, nil
}

// JoinWhere joins WHERE clause parts with AND.
func JoinWhere(parts []string) string {
	return strings.Join(parts, " AND ")
}
