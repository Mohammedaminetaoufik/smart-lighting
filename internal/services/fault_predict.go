package services

import (
	"context"
	"database/sql"
	"sync"

	"map-interactif/internal/models"
)

// FaultResult is the outcome of classifying a telemetry sample.
type FaultResult struct {
	FaultType  int     // 0 = sain, 1..4
	Label      string  // libellé humain
	Code       string  // code fault_status (none/overcurrent/overvoltage/underpower/leakage)
	Cause      string  // cause probable
	Action     string  // action recommandée
	Severity   string  // info|warning|major|critical
	AlertType  string  // type d'alerte (fault_overcurrent, ...)
	Confidence float64 // 0..1, écart au seuil
}

// faultThresholds holds the configurable detection thresholds (defaults derived
// from the fault dataset). Loaded from the fault_thresholds table.
type faultThresholdSet struct {
	overcurrentA    float64
	overvoltageV    float64
	underpowerRatio float64
	leakageA        float64
}

var (
	faultThreshMu sync.RWMutex
	faultThresh   = faultThresholdSet{overcurrentA: 4.0, overvoltageV: 233.0, underpowerRatio: 0.70, leakageA: 4.0}
)

// LoadFaultThresholds refreshes the detection thresholds from the DB. Safe to
// call at startup and after a settings change.
func LoadFaultThresholds(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM fault_thresholds`)
	if err != nil {
		return err
	}
	defer rows.Close()

	set := faultThresh // start from current/defaults
	for rows.Next() {
		var key string
		var val float64
		if err := rows.Scan(&key, &val); err != nil {
			continue
		}
		switch key {
		case "overcurrent_a":
			set.overcurrentA = val
		case "overvoltage_v":
			set.overvoltageV = val
		case "underpower_ratio":
			set.underpowerRatio = val
		case "leakage_a":
			set.leakageA = val
		}
	}
	faultThreshMu.Lock()
	faultThresh = set
	faultThreshMu.Unlock()
	return nil
}

func getFaultThresholds() faultThresholdSet {
	faultThreshMu.RLock()
	defer faultThreshMu.RUnlock()
	return faultThresh
}

// ClassifyFault detects an electrical fault in a telemetry sample using
// threshold rules learned from the fault dataset. It is a pure function
// (thresholds are cached) so it is trivially testable and safe to call inside
// the telemetry ingest transaction.
//
// Model-ready extension point: to swap in a trained classifier later, replace
// the body with a call to the prediction endpoint — callers and the return
// shape stay identical.
func ClassifyFault(lamp *models.Lampadaire, m *models.SensorMeasurement) FaultResult {
	th := getFaultThresholds()
	healthy := FaultResult{FaultType: 0, Label: "Sain", Code: "none"}
	if m == nil {
		return healthy
	}

	// 1. Surintensité — le plus dangereux (court-circuit partiel / surcharge).
	if m.Courant != nil && *m.Courant > th.overcurrentA {
		return FaultResult{
			FaultType: 1, Label: "Surintensité", Code: "overcurrent",
			Cause:  "Court-circuit partiel ou surcharge du driver.",
			Action: "Vérifier le driver et le câblage — intervention prioritaire.",
			Severity: "critical", AlertType: "fault_overcurrent",
			Confidence: confidence(*m.Courant, th.overcurrentA),
		}
	}

	// 2. Surtension — dégrade les condensateurs du driver.
	if m.Tension != nil && *m.Tension > th.overvoltageV {
		return FaultResult{
			FaultType: 2, Label: "Surtension", Code: "overvoltage",
			Cause:  "Surtension du réseau de distribution.",
			Action: "Vérifier la tension d'alimentation — risque de dégradation du driver.",
			Severity: "major", AlertType: "fault_overvoltage",
			Confidence: confidence(*m.Tension, th.overvoltageV),
		}
	}

	// 3. Sous-consommation — jugée RELATIVEMENT à la puissance attendue
	// (nominal × dimming), sinon un lampadaire atténué serait faussement signalé.
	if m.Puissance != nil && lamp != nil && lamp.NominalPowerW != nil && *lamp.NominalPowerW > 0 && lamp.Intensite > 0 {
		expected := float64(*lamp.NominalPowerW) * float64(lamp.Intensite) / 100.0
		if expected > 0 && *m.Puissance < th.underpowerRatio*expected {
			return FaultResult{
				FaultType: 3, Label: "Sous-consommation", Code: "underpower",
				Cause:  "Déplétion des LED ou driver en fin de vie.",
				Action: "Planifier le remplacement préventif du luminaire.",
				Severity: "warning", AlertType: "fault_underpower",
				Confidence: confidence(expected, *m.Puissance),
			}
		}
	}

	return healthy
}

// confidence returns a 0..1 score based on how far a value exceeds its
// threshold (capped). Deterministic stand-in for a model probability.
func confidence(value, threshold float64) float64 {
	if threshold == 0 {
		return 0.8
	}
	ratio := value / threshold
	c := 0.6 + (ratio-1.0)*0.8
	if c < 0.6 {
		c = 0.6
	}
	if c > 0.99 {
		c = 0.99
	}
	return c
}
