package repository

import (
	"context"

	"map-interactif/internal/models"
)

// InsertFaultEvent records a detected fault in the fault_events history.
// Accepts a DBExecutor so it can run inside the telemetry ingest transaction.
func InsertFaultEvent(ctx context.Context, db DBExecutor, lampID, faultType int,
	label string, confidence float64, m *models.SensorMeasurement, weather, source string) error {
	var puissance, tension, courant, temperature interface{}
	if m != nil {
		if m.Puissance != nil {
			puissance = *m.Puissance
		}
		if m.Tension != nil {
			tension = *m.Tension
		}
		if m.Courant != nil {
			courant = *m.Courant
		}
		if m.Temperature != nil {
			temperature = *m.Temperature
		}
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO fault_events
			(lampadaire_id, fault_type, label, confidence, puissance, tension, courant, temperature, weather, source)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		lampID, faultType, label, confidence, puissance, tension, courant, temperature,
		NullStringDB(weather), source)
	return err
}

// UpdateLampFaultStatus sets the current fault_status code on a lampadaire.
func UpdateLampFaultStatus(ctx context.Context, db DBExecutor, lampID int, code string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE lampadaires SET fault_status = $1, updated_at = NOW() WHERE id = $2`, code, lampID)
	return err
}
