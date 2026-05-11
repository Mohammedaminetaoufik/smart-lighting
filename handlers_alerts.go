package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// createAlertIfNotExists creates an alert only if no open alert of the same type exists for the lampadaire.
func createAlertIfNotExists(ctx context.Context, db DBExecutor, lampadaireID int, alertType string, severity string, message string) (*Alert, error) {
	// Check for existing open alert of same type
	var count int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM alerts WHERE lampadaire_id=$1 AND type=$2 AND status='open'`,
		lampadaireID, alertType).Scan(&count)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, nil // Already exists
	}

	var alert Alert
	err = db.QueryRowContext(ctx, `
		INSERT INTO alerts (lampadaire_id, type, severity, message)
		VALUES ($1, $2, $3, $4)
		RETURNING id, lampadaire_id, type, severity, message, status, created_at
	`, lampadaireID, alertType, severity, message).Scan(
		&alert.ID, &alert.LampadaireID, &alert.Type, &alert.Severity,
		&alert.Message, &alert.Status, &alert.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &alert, nil
}

// resolveOpenAlert marks an alert as resolved if it is currently open.
func resolveOpenAlert(ctx context.Context, db DBExecutor, lampadaireID int, alertType string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE alerts SET status = 'resolved', resolved_at = NOW() 
		 WHERE lampadaire_id = $1 AND type = $2 AND status = 'open'`,
		lampadaireID, alertType)
	return err
}

// runAlertRules checks telemetry data against thresholds and creates/resolves alerts.
func runAlertRules(ctx context.Context, db DBExecutor, lamp *Lampadaire, m *SensorMeasurement) []Alert {
	var alerts []Alert

	// Temperature > 75 => critical. Resolve if < 65.
	if m.Temperature != nil {
		if *m.Temperature > 75 {
			msg := fmt.Sprintf("Température élevée détectée sur le lampadaire %s (%.1f°C).", lamp.Reference, *m.Temperature)
			a, err := createAlertIfNotExists(ctx, db, lamp.ID, "temperature_elevee", "critical", msg)
			if err == nil && a != nil {
				alerts = append(alerts, *a)
			}
		} else if *m.Temperature < 65 {
			resolveOpenAlert(ctx, db, lamp.ID, "temperature_elevee")
		}
	}

	// Humidite > 85 => warning. Resolve if < 75.
	if m.Humidite != nil {
		if *m.Humidite > 85 {
			msg := fmt.Sprintf("Humidité élevée détectée sur le lampadaire %s (%.1f%%).", lamp.Reference, *m.Humidite)
			a, err := createAlertIfNotExists(ctx, db, lamp.ID, "humidite_elevee", "warning", msg)
			if err == nil && a != nil {
				alerts = append(alerts, *a)
			}
		} else if *m.Humidite < 75 {
			resolveOpenAlert(ctx, db, lamp.ID, "humidite_elevee")
		}
	}

	// Consommation anormale: puissance > lampadaire.puissance * 1.30. Resolve if < 1.20.
	if m.Puissance != nil && lamp.Puissance != nil && *lamp.Puissance > 0 {
		thresholdHigh := float64(*lamp.Puissance) * 1.30
		thresholdNormal := float64(*lamp.Puissance) * 1.20

		if *m.Puissance > thresholdHigh {
			sev := "warning"
			if *m.Puissance > float64(*lamp.Puissance)*1.50 {
				sev = "critical"
			}
			msg := fmt.Sprintf("Consommation anormale détectée sur le lampadaire %s (%.1fW vs %dW nominal).", lamp.Reference, *m.Puissance, *lamp.Puissance)
			a, err := createAlertIfNotExists(ctx, db, lamp.ID, "consommation_anormale", sev, msg)
			if err == nil && a != nil {
				alerts = append(alerts, *a)
			}
		} else if *m.Puissance < thresholdNormal {
			resolveOpenAlert(ctx, db, lamp.ID, "consommation_anormale")
		}
	}

	return alerts
}

func handleGetAlerts(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		where := []string{"1=1"}
		args := []interface{}{}
		argID := 1

		if status := c.Query("status"); status != "" {
			where = append(where, fmt.Sprintf("a.status = $%d", argID))
			args = append(args, status)
			argID++
		}
		if severity := c.Query("severity"); severity != "" {
			where = append(where, fmt.Sprintf("a.severity = $%d", argID))
			args = append(args, severity)
			argID++
		}
		if lampID := c.Query("lampadaire_id"); lampID != "" {
			where = append(where, fmt.Sprintf("a.lampadaire_id = $%d", argID))
			args = append(args, lampID)
			argID++
		}

		query := fmt.Sprintf(`
			SELECT a.id, a.lampadaire_id, a.type, a.severity, a.message, a.status, a.created_at, a.resolved_at,
				COALESCE(l.reference,'') as reference
			FROM alerts a
			LEFT JOIN lampadaires l ON a.lampadaire_id = l.id
			WHERE %s
			ORDER BY a.created_at DESC
			LIMIT 100
		`, joinWhere(where))

		rows, err := db.QueryContext(c.Request.Context(), query, args...)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur de requête.")
			return
		}
		defer rows.Close()

		alerts := []Alert{}
		for rows.Next() {
			var a Alert
			var lid sql.NullInt64
			var resolved sql.NullTime
			if err := rows.Scan(&a.ID, &lid, &a.Type, &a.Severity, &a.Message,
				&a.Status, &a.CreatedAt, &resolved, &a.Reference); err != nil {
				continue
			}
			if lid.Valid {
				v := int(lid.Int64)
				a.LampadaireID = &v
			}
			if resolved.Valid {
				a.ResolvedAt = &resolved.Time
			}
			alerts = append(alerts, a)
		}

		respondJSON(c, http.StatusOK, alerts)
	}
}

func handleResolveAlert(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "Identifiant invalide.")
			return
		}

		result, err := db.ExecContext(c.Request.Context(),
			`UPDATE alerts SET status = 'resolved', resolved_at = NOW() WHERE id = $1 AND status = 'open'`, id)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors de la résolution.")
			return
		}

		rows, _ := result.RowsAffected()
		if rows == 0 {
			respondError(c, http.StatusNotFound, "Alerte introuvable ou déjà résolue.")
			return
		}

		respondJSON(c, http.StatusOK, gin.H{"status": "resolved"})
	}
}

func handleGetAlertCounts(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var total, critical, warning, resolved int
		db.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM alerts WHERE status='open'`).Scan(&total)
		db.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM alerts WHERE status='open' AND severity='critical'`).Scan(&critical)
		db.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM alerts WHERE status='open' AND severity='warning'`).Scan(&warning)
		db.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM alerts WHERE status='resolved'`).Scan(&resolved)

		respondJSON(c, http.StatusOK, gin.H{
			"total":    total,
			"critical": critical,
			"warning":  warning,
			"resolved": resolved,
		})
	}
}

func handleGetAlertSummary(db *sql.DB) gin.HandlerFunc {
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

		respondJSON(c, http.StatusOK, gin.H{
			"open":     total,
			"critical": critical,
			"warning":  warning,
			"by_zone":  byZone,
		})
	}
}

func joinWhere(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " AND "
		}
		result += p
	}
	return result
}
