package controllers

import (
	"database/sql"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
)

// Version info — overridable via -ldflags at build time:
//
//	go build -ldflags "-X main.Commit=abc123 -X main.BuildTime=2026-05-14T12:00:00Z"
var (
	BuildCommit = "dev"
	BuildTime   = ""
)

// startedAt records process start; used for uptime.
var startedAt = time.Now()

// HandleHealth handles GET /api/health — public lightweight health probe.
func HandleHealth(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := db.PingContext(c.Request.Context()); err != nil {
			RespondError(c, http.StatusServiceUnavailable, "db unreachable")
			return
		}
		RespondJSON(c, http.StatusOK, gin.H{"status": "ok"})
	}
}

// HandleSystemHealth handles GET /api/system/health — detailed admin view.
func HandleSystemHealth(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		out := gin.H{}

		// DB ping latency
		dbStart := time.Now()
		dbErr := db.PingContext(c.Request.Context())
		out["db"] = gin.H{
			"reachable":  dbErr == nil,
			"latency_ms": time.Since(dbStart).Milliseconds(),
		}
		if dbErr != nil {
			out["db"].(gin.H)["error"] = dbErr.Error()
		}

		// Row counts for key tables
		counts := gin.H{}
		for _, t := range []string{"lampadaires", "lcus", "alerts", "work_orders", "users", "sensor_measurements", "access_logs"} {
			var n int
			_ = db.QueryRowContext(c.Request.Context(),
				"SELECT COUNT(*) FROM "+t).Scan(&n)
			counts[t] = n
		}
		out["row_counts"] = counts

		// Active alerts breakdown
		var openAlerts, criticalAlerts int
		_ = db.QueryRowContext(c.Request.Context(),
			"SELECT COUNT(*) FILTER (WHERE status='open'), COUNT(*) FILTER (WHERE status='open' AND severity='critical') FROM alerts").
			Scan(&openAlerts, &criticalAlerts)
		out["alerts"] = gin.H{"open": openAlerts, "critical_open": criticalAlerts}

		// Offline lamps (proxy for LCU sync health)
		var offlineLamps int
		_ = db.QueryRowContext(c.Request.Context(),
			"SELECT COUNT(*) FROM lampadaires WHERE etat='offline' AND archived_at IS NULL").Scan(&offlineLamps)
		out["lamps_offline"] = offlineLamps

		// Runtime
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		out["runtime"] = gin.H{
			"goroutines":  runtime.NumGoroutine(),
			"alloc_mb":    mem.Alloc / 1024 / 1024,
			"sys_mb":      mem.Sys / 1024 / 1024,
			"num_gc":      mem.NumGC,
			"uptime_secs": int(time.Since(startedAt).Seconds()),
		}

		RespondJSON(c, http.StatusOK, out)
	}
}

// HandleSystemVersion handles GET /api/system/version.
func HandleSystemVersion(c *gin.Context) {
	RespondJSON(c, http.StatusOK, gin.H{
		"commit":     BuildCommit,
		"build_time": BuildTime,
		"go_version": runtime.Version(),
		"started_at": startedAt.Format(time.RFC3339),
	})
}

// HandleSystemJobs handles GET /api/system/jobs.
func HandleSystemJobs(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.QueryContext(c.Request.Context(), "SELECT name, last_run_at, status FROM job_heartbeats ORDER BY last_run_at DESC")
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "failed to query jobs")
			return
		}
		defer rows.Close()

		jobs := []gin.H{}
		for rows.Next() {
			var name, status string
			var lastRunAt time.Time
			if err := rows.Scan(&name, &lastRunAt, &status); err == nil {
				jobs = append(jobs, gin.H{
					"name":        name,
					"last_run_at": lastRunAt.Format(time.RFC3339),
					"status":      status,
				})
			}
		}
		RespondJSON(c, http.StatusOK, jobs)
	}
}

// Default config values
var defaultSystemConfig = map[string]string{
	"alert.temp_critical_threshold":   "75",
	"alert.power_abnormal_multiplier": "1.3",
	"job.offline_check_interval_min":  "1",
	"job.telemetry_retention_days":    "90",
	"lcu.sync_interval_min":           "5",
}

// HandleGetSystemConfig handles GET /api/system/config.
func HandleGetSystemConfig(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.QueryContext(c.Request.Context(), "SELECT key, value FROM system_settings")
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "failed to query config")
			return
		}
		defer rows.Close()

		config := make(map[string]string)
		for k, v := range defaultSystemConfig {
			config[k] = v // load defaults first
		}

		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err == nil {
				config[k] = v
			}
		}

		RespondJSON(c, http.StatusOK, config)
	}
}

// HandleUpdateSystemConfig handles PUT /api/system/config.
func HandleUpdateSystemConfig(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body map[string]string
		if !BindRequiredJSON(c, &body) {
			return
		}

		tx, err := db.BeginTx(c.Request.Context(), nil)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "tx error")
			return
		}
		defer tx.Rollback()

		stmt, err := tx.PrepareContext(c.Request.Context(), `
			INSERT INTO system_settings (key, value, updated_at) 
			VALUES ($1, $2, NOW()) 
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
		`)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "stmt error")
			return
		}
		defer stmt.Close()

		for k, v := range body {
			if _, err := stmt.ExecContext(c.Request.Context(), k, v); err != nil {
				RespondError(c, http.StatusInternalServerError, "failed to update config")
				return
			}
		}

		if err := tx.Commit(); err != nil {
			RespondError(c, http.StatusInternalServerError, "commit error")
			return
		}

		RespondJSON(c, http.StatusOK, gin.H{"status": "ok"})
	}
}
