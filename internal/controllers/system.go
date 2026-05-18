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

// JobHeartbeat represents a background job status.
type JobHeartbeat struct {
	Name      string    `json:"name"`
	LastRunAt time.Time `json:"last_run_at"`
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
}

// HandleSystemJobs handles GET /api/system/jobs.
func HandleSystemJobs(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT name, last_run_at, status, message 
			FROM job_heartbeats 
			ORDER BY name
		`)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Impossible de charger les tâches : "+err.Error())
			return
		}
		defer rows.Close()

		jobs := []JobHeartbeat{}
		for rows.Next() {
			var job JobHeartbeat
			var msg sql.NullString
			if err := rows.Scan(&job.Name, &job.LastRunAt, &job.Status, &msg); err != nil {
				RespondError(c, http.StatusInternalServerError, "Erreur de lecture de tâche : "+err.Error())
				return
			}
			if msg.Valid {
				job.Message = msg.String
			}
			jobs = append(jobs, job)
		}

		RespondJSON(c, http.StatusOK, jobs)
	}
}

// HandleGetSystemConfig handles GET /api/system/config.
func HandleGetSystemConfig(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		defaults := map[string]string{
			"alert.temp_critical_threshold":   "85",
			"alert.power_abnormal_multiplier": "1.5",
			"job.offline_check_interval_min":   "15",
			"job.telemetry_retention_days":     "90",
			"lcu.sync_interval_min":           "10",
		}

		rows, err := db.QueryContext(c.Request.Context(), "SELECT key, value FROM system_settings")
		if err != nil {
			// If table doesn't exist yet, we still return the defaults rather than failing
			RespondJSON(c, http.StatusOK, defaults)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err == nil {
				defaults[k] = v
			}
		}

		RespondJSON(c, http.StatusOK, defaults)
	}
}

// HandleUpdateSystemConfig handles PUT /api/system/config.
func HandleUpdateSystemConfig(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req map[string]string
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondError(c, http.StatusBadRequest, "Corps de requête invalide")
			return
		}

		tx, err := db.BeginTx(c.Request.Context(), nil)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Impossible de démarrer la transaction : "+err.Error())
			return
		}
		defer tx.Rollback()

		stmt, err := tx.PrepareContext(c.Request.Context(), `
			INSERT INTO system_settings (key, value, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
		`)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur de préparation : "+err.Error())
			return
		}
		defer stmt.Close()

		for k, v := range req {
			if _, err := stmt.ExecContext(c.Request.Context(), k, v); err != nil {
				RespondError(c, http.StatusInternalServerError, "Erreur d'enregistrement : "+err.Error())
				return
			}
		}

		if err := tx.Commit(); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors de la validation : "+err.Error())
			return
		}

		RespondJSON(c, http.StatusOK, gin.H{"status": "ok"})
	}
}
