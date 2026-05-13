package main

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

func handleGetNetworkHealth(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		type NetworkHealth struct {
			TotalBasestations       int     `json:"total_basestations"`
			BasestationsOnline      int     `json:"basestations_online"`
			BasestationsOffline     int     `json:"basestations_offline"`
			TotalControllers        int     `json:"total_controllers"`
			ControllersOK           int     `json:"controllers_ok"`
			ControllersLost         int     `json:"controllers_lost"`
			AvgSignalQuality        float64 `json:"avg_signal_quality"`
			NetworkAvailabilityPct  float64 `json:"network_availability_pct"`
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
		respondJSON(c, http.StatusOK, h)
	}
}

func handleGetCommissioningProgress(db *sql.DB) gin.HandlerFunc {
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
			respondError(c, http.StatusInternalServerError, "Database error")
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
		respondJSON(c, http.StatusOK, gin.H{
			"steps":              steps,
			"total":              total,
			"commissioned":       commissioned,
			"commissioning_rate": rate,
		})
	}
}

// enrichDashboardStats adds new entity counts to the existing stats object.
func enrichDashboardStats(ctx context.Context, db *sql.DB, stats *DashboardStats) {
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM basestations`).Scan(&stats.TotalBasestations)
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM basestations WHERE status='online'`).Scan(&stats.BasestationsOnline)
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cabinets`).Scan(&stats.TotalCabinets)
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM controllers`).Scan(&stats.TotalControllers)
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM work_orders WHERE status NOT IN ('closed','cancelled','resolved')`).Scan(&stats.OpenWorkOrders)
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM work_orders WHERE priority='urgent' AND status NOT IN ('closed','cancelled','resolved')`).Scan(&stats.UrgentWorkOrders)
	var total, commissioned int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM lampadaires WHERE archived_at IS NULL`).Scan(&total)
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM lampadaires WHERE archived_at IS NULL AND commissioning_status='commissioned'`).Scan(&commissioned)
	if total > 0 {
		stats.CommissioningRate = float64(commissioned) / float64(total) * 100
	}
}
