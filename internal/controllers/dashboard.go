package controllers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/repository"
)

// HandleGetDashboardStats handles GET /api/dashboard/stats.
func HandleGetDashboardStats(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		stats, err := repository.GetDashboardStats(c.Request.Context(), db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors du chargement des statistiques.")
			return
		}
		repository.EnrichDashboardStats(c.Request.Context(), db, stats)
		RespondJSON(c, http.StatusOK, stats)
	}
}

// HandleGetEnergySummary handles GET /api/energy/summary.
func HandleGetEnergySummary(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		summary, err := repository.GetEnergySummary(c.Request.Context(), db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors du calcul énergétique.")
			return
		}
		RespondJSON(c, http.StatusOK, summary)
	}
}

// HandleGetDailyEnergy handles GET /api/energy/daily?days=30
// Returns estimated daily kWh from sensor_measurements for the last N days.
func HandleGetDailyEnergy(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		days := 30
		if d, err := strconv.Atoi(c.Query("days")); err == nil && d > 0 && d <= 365 {
			days = d
		}

		type DayEnergy struct {
			Date string  `json:"date"`
			KWh  float64 `json:"kwh"`
		}

		// Sum energy field if populated; otherwise estimate from puissance readings
		// assuming ~5-minute measurement intervals (5/60 h per record).
		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT
				DATE(created_at)::text AS day,
				COALESCE(
					NULLIF(SUM(energie), 0),
					SUM(puissance) * 5.0 / 60.0 / 1000.0
				) AS kwh
			FROM sensor_measurements
			WHERE created_at >= NOW() - ($1::text || ' days')::interval
			  AND (energie IS NOT NULL OR puissance IS NOT NULL)
			GROUP BY DATE(created_at)
			ORDER BY day
		`, days)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		// Build a full 30-day series filling missing days with 0
		result := make([]DayEnergy, 0, days)
		byDate := map[string]float64{}
		for rows.Next() {
			var d DayEnergy
			if err := rows.Scan(&d.Date, &d.KWh); err == nil {
				byDate[d.Date] = d.KWh
			}
		}

		for i := days - 1; i >= 0; i-- {
			day := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
			result = append(result, DayEnergy{Date: day, KWh: byDate[day]})
		}

		RespondJSON(c, http.StatusOK, result)
	}
}

// HandleGetNetworkHealth handles GET /api/dashboard/network-health.
func HandleGetNetworkHealth(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		type NetworkHealth struct {
			TotalBasestations      int     `json:"total_basestations"`
			BasestationsOnline     int     `json:"basestations_online"`
			BasestationsOffline    int     `json:"basestations_offline"`
			TotalControllers       int     `json:"total_controllers"`
			ControllersOK          int     `json:"controllers_ok"`
			ControllersLost        int     `json:"controllers_lost"`
			AvgSignalQuality       float64 `json:"avg_signal_quality"`
			NetworkAvailabilityPct float64 `json:"network_availability_pct"`
		}
		var h NetworkHealth
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM basestations`).Scan(&h.TotalBasestations)
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM basestations WHERE status='online'`).Scan(&h.BasestationsOnline)
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM basestations WHERE status='offline'`).Scan(&h.BasestationsOffline)
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM controllers`).Scan(&h.TotalControllers)
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM controllers WHERE communication_status='ok'`).Scan(&h.ControllersOK)
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM controllers WHERE communication_status='lost'`).Scan(&h.ControllersLost)
		db.QueryRowContext(ctx, `SELECT COALESCE(AVG(signal_quality),0) FROM controllers`).Scan(&h.AvgSignalQuality)
		if h.TotalBasestations > 0 {
			h.NetworkAvailabilityPct = float64(h.BasestationsOnline) / float64(h.TotalBasestations) * 100
		}
		RespondJSON(c, http.StatusOK, h)
	}
}

// HandleGetCommissioningProgress handles GET /api/dashboard/commissioning-progress.
func HandleGetCommissioningProgress(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		type StepCount struct {
			Status string `json:"status"`
			Count  int    `json:"count"`
		}
		rows, err := db.QueryContext(ctx, `
			SELECT commissioning_status, COUNT(*) as cnt
			FROM lampadaires WHERE archived_at IS NULL
			GROUP BY commissioning_status ORDER BY commissioning_status`)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()
		var steps []StepCount
		total := 0
		commissioned := 0
		for rows.Next() {
			var s StepCount
			if err := rows.Scan(&s.Status, &s.Count); err == nil {
				steps = append(steps, s)
				total += s.Count
				if s.Status == "commissioned" {
					commissioned = s.Count
				}
			}
		}
		rate := 0.0
		if total > 0 {
			rate = float64(commissioned) / float64(total) * 100
		}
		RespondJSON(c, http.StatusOK, gin.H{
			"steps":              steps,
			"total":              total,
			"commissioned":       commissioned,
			"commissioning_rate": rate,
		})
	}
}
