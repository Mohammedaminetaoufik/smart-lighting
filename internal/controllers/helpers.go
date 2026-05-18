package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	errInvalidJSON = "JSON invalide: "
	errInvalidID   = "ID invalide"
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

// BindOptionalJSON binds a JSON body if present, returning false (and writing 400)
// when the body is non-empty but malformed. An empty body is treated as success
// so callers can supply default values in `obj`.
func BindOptionalJSON(c *gin.Context, obj interface{}) bool {
	if c.Request.ContentLength <= 0 {
		return true
	}
	if err := c.ShouldBindJSON(obj); err != nil {
		RespondError(c, http.StatusBadRequest, errInvalidJSON+err.Error())
		return false
	}
	return true
}

// BindRequiredJSON binds a required JSON body, returning false (and writing 400)
// on any parsing error including an empty body.
func BindRequiredJSON(c *gin.Context, obj interface{}) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		RespondError(c, http.StatusBadRequest, errInvalidJSON+err.Error())
		return false
	}
	return true
}
