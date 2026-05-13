package controllers

import (
	"database/sql"
	"net/http"

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
