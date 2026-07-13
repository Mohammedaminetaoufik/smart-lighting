package services

import (
	"context"
	"database/sql"
	"fmt"

	"map-interactif/internal/models"
	"map-interactif/internal/repository"
)

// RunAlertRules checks telemetry data against thresholds and creates/resolves alerts.
// Alerts are suppressed (if non-critical) when the lamp is in an active maintenance window
// that has suppress_alerts=true. All created alerts inside a maintenance window are
// flagged maintenance_related=true regardless of suppression.
func RunAlertRules(ctx context.Context, db repository.DBExecutor, lamp *models.Lampadaire, m *models.SensorMeasurement) []models.Alert {
	var alerts []models.Alert

	inMaint, win := IsEquipmentInMaintenance(ctx, db, lamp.ID, lamp.Zone, lamp.LCUID)

	// createAlert wraps CreateAlertIfNotExists with maintenance logic.
	// isCritical=true means the alert is never suppressed even during maintenance.
	createAlert := func(alertType, severity, message string, isCritical bool) {
		// Suppress non-critical alerts when maintenance window has suppress_alerts.
		if inMaint && win.SuppressAlerts && !isCritical {
			return
		}
		a, err := repository.CreateAlertIfNotExists(ctx, db, lamp.ID, alertType, severity, message)
		if err != nil || a == nil {
			return
		}
		// Tag alert as maintenance-related when inside a window.
		if inMaint {
			if sqlDB, ok := db.(*sql.DB); ok {
				MarkAlertMaintenanceRelated(ctx, sqlDB, a.ID, win.ID)
			}
			a.MaintenanceRelated = true
		}
		alerts = append(alerts, *a)
	}

	// Temperature > 75 => critical. Resolve if < 65.
	if m.Temperature != nil {
		if *m.Temperature > 75 {
			msg := fmt.Sprintf("Température élevée détectée sur le lampadaire %s (%.1f°C).", lamp.Reference, *m.Temperature)
			createAlert("temperature_elevee", "critical", msg, true)
		} else if *m.Temperature < 65 {
			repository.ResolveOpenAlert(ctx, db, lamp.ID, "temperature_elevee")
		}
	}

	// Humidite > 85 => warning. Resolve if < 75.
	if m.Humidite != nil {
		if *m.Humidite > 85 {
			msg := fmt.Sprintf("Humidité élevée détectée sur le lampadaire %s (%.1f%%).", lamp.Reference, *m.Humidite)
			createAlert("humidite_elevee", "warning", msg, false)
		} else if *m.Humidite < 75 {
			repository.ResolveOpenAlert(ctx, db, lamp.ID, "humidite_elevee")
		}
	}

	// Consommation anormale: puissance > lampadaire.puissance * 1.30. Resolve if < 1.20.
	if m.Puissance != nil && lamp.Puissance != nil && *lamp.Puissance > 0 {
		thresholdHigh := float64(*lamp.Puissance) * 1.30
		thresholdNormal := float64(*lamp.Puissance) * 1.20

		if *m.Puissance > thresholdHigh {
			sev := "warning"
			isCritical := false
			if *m.Puissance > float64(*lamp.Puissance)*1.50 {
				sev = "critical"
				isCritical = true
			}
			msg := fmt.Sprintf("Consommation anormale détectée sur le lampadaire %s (%.1fW vs %dW nominal).", lamp.Reference, *m.Puissance, *lamp.Puissance)
			createAlert("consommation_anormale", sev, msg, isCritical)
		} else if *m.Puissance < thresholdNormal {
			repository.ResolveOpenAlert(ctx, db, lamp.ID, "consommation_anormale")
		}
	}

	// Maintenance prédictive : classer la panne électrique (règles apprises du
	// dataset). Sur panne → alerte + historique fault_events + fault_status.
	if fr := ClassifyFault(lamp, m); fr.FaultType != 0 {
		msg := fmt.Sprintf("%s détectée sur le lampadaire %s (%s).", fr.Label, lamp.Reference, fr.Cause)
		createAlert(fr.AlertType, fr.Severity, msg, fr.Severity == "critical")
		_ = repository.InsertFaultEvent(ctx, db, lamp.ID, fr.FaultType, fr.Label, fr.Confidence, m, "", "live")
		_ = repository.UpdateLampFaultStatus(ctx, db, lamp.ID, fr.Code)
	} else {
		// Sain : résoudre les alertes de panne live (on préserve le fault_status
		// issu du backfill dataset — non touché ici).
		for _, at := range []string{"fault_overcurrent", "fault_overvoltage", "fault_underpower"} {
			repository.ResolveOpenAlert(ctx, db, lamp.ID, at)
		}
	}

	return alerts
}
