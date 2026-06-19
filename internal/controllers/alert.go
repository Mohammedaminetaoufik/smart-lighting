package controllers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/repository"
	"map-interactif/internal/services"
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
			`UPDATE alerts SET status = 'resolved', resolved_at = NOW() WHERE id = $1 AND status NOT IN ('resolved','closed')`, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors de la résolution.")
			return
		}

		rows, _ := result.RowsAffected()
		if rows == 0 {
			RespondError(c, http.StatusNotFound, "Alerte introuvable ou déjà résolue.")
			return
		}
		acR := services.GetAuditContext(c)
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: acR.UserID, UserName: acR.UserName, UserRole: acR.UserRole,
			Action: "alert_resolved", EntityType: "alert", EntityID: &id,
			Description: "Alerte résolue",
			NewValues: map[string]any{"status": "resolved"},
			IPAddress: acR.IPAddress, UserAgent: acR.UserAgent,
		})
		RespondJSON(c, http.StatusOK, gin.H{"status": "resolved"})
	}
}

// HandleGetAlertCounts handles GET /api/alerts/counts.
func HandleGetAlertCounts(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var total, critical, warning, resolved int
		err := db.QueryRowContext(c.Request.Context(), `
			SELECT
				COUNT(*) FILTER (WHERE status='open'),
				COUNT(*) FILTER (WHERE status='open' AND severity='critical'),
				COUNT(*) FILTER (WHERE status='open' AND severity='warning'),
				COUNT(*) FILTER (WHERE status='resolved')
			FROM alerts
		`).Scan(&total, &critical, &warning, &resolved)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur de chargement des compteurs")
			return
		}

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
		if err := db.QueryRowContext(c.Request.Context(), `
			SELECT
				COUNT(*) FILTER (WHERE status='open'),
				COUNT(*) FILTER (WHERE status='open' AND severity='critical'),
				COUNT(*) FILTER (WHERE status='open' AND severity='warning')
			FROM alerts
		`).Scan(&total, &critical, &warning); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur de chargement du résumé")
			return
		}

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

// HandleGetAlertTimeline handles GET /api/alerts/timeline.
// Returns the count of alerts created per hour over the last 24 hours.
func HandleGetAlertTimeline(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		type HourBucket struct {
			HoursAgo int `json:"hours_ago"`
			Count    int `json:"count"`
			Critical int `json:"critical"`
		}

		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT
				FLOOR(EXTRACT(EPOCH FROM (NOW() - created_at)) / 3600)::int AS hours_ago,
				COUNT(*) AS cnt,
				COUNT(*) FILTER (WHERE severity = 'critical') AS crit
			FROM alerts
			WHERE created_at >= NOW() - INTERVAL '24 hours'
			GROUP BY hours_ago
		`)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		byHour := map[int]HourBucket{}
		for rows.Next() {
			var b HourBucket
			if rows.Scan(&b.HoursAgo, &b.Count, &b.Critical) == nil {
				byHour[b.HoursAgo] = b
			}
		}

		// Build a full 24-slot series, oldest first (23h ago → now)
		result := make([]HourBucket, 0, 24)
		for h := 23; h >= 0; h-- {
			if b, ok := byHour[h]; ok {
				result = append(result, b)
			} else {
				result = append(result, HourBucket{HoursAgo: h, Count: 0, Critical: 0})
			}
		}
		RespondJSON(c, http.StatusOK, result)
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
		ac := services.GetAuditContext(c)
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: ac.UserID, UserName: ac.UserName, UserRole: ac.UserRole,
			Action: "alert_acknowledged", EntityType: "alert", EntityID: &id,
			Description: "Alerte acquittée",
			NewValues: map[string]any{"status": "acknowledged"},
			IPAddress: ac.IPAddress, UserAgent: ac.UserAgent,
		})
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
		acC := services.GetAuditContext(c)
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: acC.UserID, UserName: acC.UserName, UserRole: acC.UserRole,
			Action: "alert_closed", EntityType: "alert", EntityID: &id,
			Description: "Alerte fermée",
			NewValues: map[string]any{"status": "closed"},
			IPAddress: acC.IPAddress, UserAgent: acC.UserAgent,
		})
		RespondJSON(c, http.StatusOK, gin.H{"status": "closed", "alert_id": id})
	}
}

// HandleCreateWorkOrderFromAlert handles POST /api/alerts/:id/create-work-order.
// Implements the smart creation logic: dedup, diagnose, link.
func HandleCreateWorkOrderFromAlert(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}

		wo, existed, err := services.CreateWorkOrderFromAlert(db, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur création bon de travail: "+err.Error())
			return
		}

		status := http.StatusCreated
		if existed {
			status = http.StatusOK
		}
		acWO := services.GetAuditContext(c)
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: acWO.UserID, UserName: acWO.UserName, UserRole: acWO.UserRole,
			Action: "work_order_created_from_alert", EntityType: "alert", EntityID: &id,
			Description: messageForWO(existed),
			NewValues: map[string]any{"work_order_id": wo.ID, "existed": existed},
			IPAddress: acWO.IPAddress, UserAgent: acWO.UserAgent,
		})
		RespondJSON(c, status, gin.H{
			"work_order": wo,
			"existed":    existed,
			"message":    messageForWO(existed),
		})
	}
}

func messageForWO(existed bool) string {
	if existed {
		return "Alerte liée au bon de travail existant"
	}
	return "Bon de travail créé"
}
