package services

import (
	"context"
	"fmt"

	"map-interactif/internal/models"
	"map-interactif/internal/repository"
)

// RunAlertRules checks telemetry data against thresholds and creates/resolves alerts.
func RunAlertRules(ctx context.Context, db repository.DBExecutor, lamp *models.Lampadaire, m *models.SensorMeasurement) []models.Alert {
	var alerts []models.Alert

	// Temperature > 75 => critical. Resolve if < 65.
	if m.Temperature != nil {
		if *m.Temperature > 75 {
			msg := fmt.Sprintf("Température élevée détectée sur le lampadaire %s (%.1f°C).", lamp.Reference, *m.Temperature)
			a, err := repository.CreateAlertIfNotExists(ctx, db, lamp.ID, "temperature_elevee", "critical", msg)
			if err == nil && a != nil {
				alerts = append(alerts, *a)
			}
		} else if *m.Temperature < 65 {
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
