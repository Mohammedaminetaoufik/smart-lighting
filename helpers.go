package main

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// DBExecutor defines common database operations for both *sql.DB and *sql.Tx.
type DBExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// respondJSON sends a JSON response with the given status code.
func respondJSON(c *gin.Context, status int, data interface{}) {
	c.JSON(status, data)
}

// respondError sends a JSON error response.
func respondError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

// parseIDParam parses an integer ID from a Gin URL parameter.
func parseIDParam(c *gin.Context, name string) (int, error) {
	id, err := strconv.Atoi(c.Param(name))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("identifiant invalide")
	}
	return id, nil
}

// clampIntensity ensures intensity is between 0 and 100.
func clampIntensity(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// nullString returns nil for empty strings, or the string value.
func nullString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

// parseOptionalInt parses an optional integer string.
func parseOptionalInt(value string) (*int, error) {
	if value == "" {
		return nil, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return nil, fmt.Errorf("invalid")
	}

	return &parsed, nil
}

// parseOptionalDate parses an optional date string in YYYY-MM-DD format.
func parseOptionalDate(value string) (*string, error) {
	if value == "" {
		return nil, nil
	}

	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, err
	}

	formatted := parsed.Format("2006-01-02")
	return &formatted, nil
}

// ptrFloat64 returns a pointer to a float64 value.
func ptrFloat64(v float64) *float64 {
	return &v
}

// ptrBool returns a pointer to a bool value.
func ptrBool(v bool) *bool {
	return &v
}

// minInt returns the smaller of two integers.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func fillDashboardStats(ctx context.Context, db *sql.DB, stats *DashboardStats) error {
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lcus").Scan(&stats.TotalLCUs)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lcus WHERE status='online'").Scan(&stats.LCUsOnline)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lcus WHERE status='offline'").Scan(&stats.LCUsOffline)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lampadaires WHERE archived_at IS NULL").Scan(&stats.TotalLampadaires)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lampadaires WHERE archived_at IS NULL AND etat='online'").Scan(&stats.LampadairesOnline)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lampadaires WHERE archived_at IS NULL AND etat='offline'").Scan(&stats.LampadairesOffline)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lampadaires WHERE archived_at IS NULL AND etat='maintenance'").Scan(&stats.LampadairesMaintenance)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lampadaires WHERE archived_at IS NULL AND (latitude=0 OR latitude IS NULL) AND location_status='missing'").Scan(&stats.MissingLocation)
	return nil
}
