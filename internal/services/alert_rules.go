package services

import (
	"context"
	"fmt"
	"strconv"

	"map-interactif/internal/models"
	"map-interactif/internal/repository"
)

// RunAlertRules checks telemetry data against thresholds and creates/resolves alerts.
func RunAlertRules(ctx context.Context, db repository.DBExecutor, lamp *models.Lampadaire, m *models.SensorMeasurement) []models.Alert {
	var alerts []models.Alert

	// 1. Check if under maintenance
	var underMaintenance bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM maintenance_windows 
			WHERE starts_at <= NOW() AND ends_at >= NOW()
			AND (zone = $1 OR $2 = ANY(lampadaire_ids))
		)
	`, lamp.Zone, lamp.ID).Scan(&underMaintenance)
	if err == nil && underMaintenance {
		// Suppress new alerts during maintenance. Existing open alerts could optionally be resolved, but just returning nil is fine.
		return alerts
	}

	// 2. Fetch thresholds
	tempCritical := 75.0
	powerMult := 1.30
	rows, _ := db.QueryContext(ctx, "SELECT key, value FROM system_settings WHERE key IN ('alert.temp_critical_threshold', 'alert.power_abnormal_multiplier')")
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var k, v string
			if rows.Scan(&k, &v) == nil {
				if k == "alert.temp_critical_threshold" {
					if val, err := strconv.ParseFloat(v, 64); err == nil {
						tempCritical = val
					}
				} else if k == "alert.power_abnormal_multiplier" {
					if val, err := strconv.ParseFloat(v, 64); err == nil {
						powerMult = val
					}
				}
			}
		}
	}

	// Temperature > threshold => critical. Resolve if < threshold - 10.
	if m.Temperature != nil {
		if *m.Temperature > tempCritical {
			msg := fmt.Sprintf("Température élevée détectée sur le lampadaire %s (%.1f°C).", lamp.Reference, *m.Temperature)
			a, err := repository.CreateAlertIfNotExists(ctx, db, lamp.ID, "temperature_elevee", "critical", msg)
			if err == nil && a != nil {
				alerts = append(alerts, *a)
			}
		} else if *m.Temperature < tempCritical-10.0 {
			repository.ResolveOpenAlert(ctx, db, lamp.ID, "temperature_elevee")
		}
	}

	// Humidite > 85 => warning. Resolve if < 75.
	if m.Humidite != nil {
		if *m.Humidite > 85 {
			msg := fmt.Sprintf("Humidité élevée détectée sur le lampadaire %s (%.1f%%).", lamp.Reference, *m.Humidite)
			a, err := repository.CreateAlertIfNotExists(ctx, db, lamp.ID, "humidite_elevee", "warning", msg)
			if err == nil && a != nil {
				alerts = append(alerts, *a)
			}
		} else if *m.Humidite < 75 {
			repository.ResolveOpenAlert(ctx, db, lamp.ID, "humidite_elevee")
		}
	}

	// Consommation anormale: puissance > lampadaire.puissance * powerMult.
	if m.Puissance != nil && lamp.Puissance != nil && *lamp.Puissance > 0 {
		thresholdHigh := float64(*lamp.Puissance) * powerMult
		thresholdNormal := float64(*lamp.Puissance) * (powerMult - 0.10)

		if *m.Puissance > thresholdHigh {
			sev := "warning"
			if *m.Puissance > float64(*lamp.Puissance)*(powerMult+0.20) {
				sev = "critical"
			}
			msg := fmt.Sprintf("Consommation anormale détectée sur le lampadaire %s (%.1fW vs %dW nominal).", lamp.Reference, *m.Puissance, *lamp.Puissance)
			a, err := repository.CreateAlertIfNotExists(ctx, db, lamp.ID, "consommation_anormale", sev, msg)
			if err == nil && a != nil {
				alerts = append(alerts, *a)
			}
		} else if *m.Puissance < thresholdNormal {
			repository.ResolveOpenAlert(ctx, db, lamp.ID, "consommation_anormale")
		}
	}

	return alerts
}

