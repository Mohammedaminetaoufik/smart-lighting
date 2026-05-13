package controllers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/repository"
)

// HandleGetAlerts handles GET /api/alerts.
func HandleGetAlerts(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		filters := map[string]string{
			"status":        c.Query("status"),
			"severity":      c.Query("severity"),
			"lampadaire_id": c.Query("lampadaire_id"),
		}
		alerts, err := repository.ListAlerts(c.Request.Context(), db, filters)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur de requête.")
			return
		}
		RespondJSON(c, http.StatusOK, alerts)
	}
}

// HandleResolveAlert handles POST /api/alerts/:id/resolve.
func HandleResolveAlert(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "Identifiant invalide.")
			return
		}

		result, err := db.ExecContext(c.Request.Context(),
			`UPDATE alerts SET status = 'resolved', resolved_at = NOW() WHERE id = $1 AND status = 'open'`, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors de la résolution.")
			return
		}

		rows, _ := result.RowsAffected()
		if rows == 0 {
			RespondError(c, http.StatusNotFound, "Alerte introuvable ou déjà résolue.")
			return
		}

		RespondJSON(c, http.StatusOK, gin.H{"status": "resolved"})
	}
}

// HandleGetAlertCounts handles GET /api/alerts/counts.
func HandleGetAlertCounts(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var total, critical, warning, resolved int
		db.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM alerts WHERE status='open'`).Scan(&total)
		db.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM alerts WHERE status='open' AND severity='critical'`).Scan(&critical)
		db.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM alerts WHERE status='open' AND severity='warning'`).Scan(&warning)
		db.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM alerts WHERE status='resolved'`).Scan(&resolved)

		RespondJSON(c, http.StatusOK, gin.H{
			"total":    total,
			"critical": critical,
			"warning":  warning,
			"resolved": resolved,
		})
	}
}

// HandleGetAlertSummary handles GET /api/alerts/summary.
func HandleGetAlertSummary(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var total, critical, warning int
		db.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM alerts WHERE status='open'`).Scan(&total)
		db.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM alerts WHERE status='open' AND severity='critical'`).Scan(&critical)
		db.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM alerts WHERE status='open' AND severity='warning'`).Scan(&warning)

		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT COALESCE(l.zone, 'Inconnue'), COUNT(*)
			FROM alerts a JOIN lampadaires l ON a.lampadaire_id = l.id
			WHERE a.status = 'open'
			GROUP BY l.zone
		`)
		byZone := []gin.H{}
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var zone string
				var count int
				if err := rows.Scan(&zone, &count); err == nil {
					byZone = append(byZone, gin.H{"zone": zone, "count": count})
				}
			}
		}

		RespondJSON(c, http.StatusOK, gin.H{
			"open":     total,
			"critical": critical,
			"warning":  warning,
			"by_zone":  byZone,
		})
	}
}

// HandleAckAlert handles POST /api/alerts/:id/ack.
func HandleAckAlert(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		_, err = db.Exec(`
			UPDATE alerts SET status='acknowledged', acknowledged_at=NOW()
			WHERE id=$1 AND status='open'`, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		RespondJSON(c, http.StatusOK, gin.H{"status": "acknowledged", "alert_id": id})
	}
}

// HandleCloseAlert handles POST /api/alerts/:id/close.
func HandleCloseAlert(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		_, err = db.Exec(`
			UPDATE alerts SET status='closed', closed_at=NOW()
			WHERE id=$1 AND status NOT IN ('closed')`, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		RespondJSON(c, http.StatusOK, gin.H{"status": "closed", "alert_id": id})
	}
}

