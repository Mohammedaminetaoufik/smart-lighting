package controllers

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// faultLabels maps fault_status codes to human labels.
var faultLabels = map[string]string{
	"overcurrent": "Surintensité",
	"overvoltage": "Surtension",
	"underpower":  "Sous-consommation",
	"leakage":     "Fuite de courant",
	"overtemp":    "Surchauffe",
	"none":        "Sain",
}

// HandleGetAtRiskLamps handles GET /api/faults/at-risk.
// Returns the current watchlist: lampadaires with a fault_status, their fault
// count and last fault date.
func HandleGetAtRiskLamps(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT l.id, COALESCE(l.reference, l.id::text) AS reference,
			       COALESCE(l.zone, 'Sans zone') AS zone, l.fault_status,
			       COALESCE(fc.n, 0) AS fault_count, fc.last_at
			FROM lampadaires l
			LEFT JOIN (
				SELECT lampadaire_id, COUNT(*) AS n, MAX(created_at) AS last_at
				FROM fault_events GROUP BY lampadaire_id
			) fc ON fc.lampadaire_id = l.id
			WHERE l.archived_at IS NULL AND l.fault_status IS NOT NULL AND l.fault_status <> 'none'
			ORDER BY fault_count DESC, reference`)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur requête")
			return
		}
		defer rows.Close()

		type atRisk struct {
			ID          int        `json:"id"`
			Reference   string     `json:"reference"`
			Zone        string     `json:"zone"`
			FaultStatus string     `json:"fault_status"`
			FaultLabel  string     `json:"fault_label"`
			FaultCount  int        `json:"fault_count"`
			LastFaultAt *time.Time `json:"last_fault_at"`
		}
		out := []atRisk{}
		for rows.Next() {
			var a atRisk
			var last sql.NullTime
			if err := rows.Scan(&a.ID, &a.Reference, &a.Zone, &a.FaultStatus, &a.FaultCount, &last); err == nil {
				a.FaultLabel = faultLabels[a.FaultStatus]
				if a.FaultLabel == "" {
					a.FaultLabel = a.FaultStatus
				}
				if last.Valid {
					a.LastFaultAt = &last.Time
				}
				out = append(out, a)
			}
		}
		RespondJSON(c, http.StatusOK, out)
	}
}

// HandleGetFaultStats handles GET /api/faults/stats.
// Returns fault distribution by type + at-risk / healthy counts.
func HandleGetFaultStats(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		type typeCount struct {
			FaultType int    `json:"fault_type"`
			Label     string `json:"label"`
			Count     int    `json:"count"`
		}
		byType := []typeCount{}
		rows, err := db.QueryContext(ctx, `
			SELECT fault_type, label, COUNT(*) FROM fault_events
			GROUP BY fault_type, label ORDER BY fault_type`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var t typeCount
				if rows.Scan(&t.FaultType, &t.Label, &t.Count) == nil {
					byType = append(byType, t)
				}
			}
		}

		var atRisk, healthy, totalEvents int
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM lampadaires WHERE archived_at IS NULL AND fault_status IS NOT NULL AND fault_status <> 'none'`).Scan(&atRisk)
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM lampadaires WHERE archived_at IS NULL AND (fault_status IS NULL OR fault_status = 'none')`).Scan(&healthy)
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fault_events`).Scan(&totalEvents)

		RespondJSON(c, http.StatusOK, gin.H{
			"by_type":      byType,
			"at_risk":      atRisk,
			"healthy":      healthy,
			"total_events": totalEvents,
		})
	}
}

// HandleGetLampFaults handles GET /api/lampadaires/:id/faults.
// Returns the fault history for one lampadaire.
func HandleGetLampFaults(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "Identifiant invalide")
			return
		}
		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT fault_type, label, weather, puissance, tension, courant, created_at
			FROM fault_events WHERE lampadaire_id = $1
			ORDER BY created_at DESC LIMIT 50`, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur requête")
			return
		}
		defer rows.Close()

		type faultRow struct {
			FaultType int       `json:"fault_type"`
			Label     string    `json:"label"`
			Weather   *string   `json:"weather"`
			Puissance *float64  `json:"puissance"`
			Tension   *float64  `json:"tension"`
			Courant   *float64  `json:"courant"`
			CreatedAt time.Time `json:"created_at"`
		}
		out := []faultRow{}
		for rows.Next() {
			var f faultRow
			var w sql.NullString
			var p, t, cu sql.NullFloat64
			if err := rows.Scan(&f.FaultType, &f.Label, &w, &p, &t, &cu, &f.CreatedAt); err == nil {
				if w.Valid {
					f.Weather = &w.String
				}
				if p.Valid {
					f.Puissance = &p.Float64
				}
				if t.Valid {
					f.Tension = &t.Float64
				}
				if cu.Valid {
					f.Courant = &cu.Float64
				}
				out = append(out, f)
			}
		}
		RespondJSON(c, http.StatusOK, out)
	}
}
