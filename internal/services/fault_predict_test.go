package services

import (
	"testing"

	"map-interactif/internal/models"
)

func fptr(v float64) *float64 { return &v }
func iptr(v int) *int         { return &v }

func TestClassifyFault(t *testing.T) {
	// Lampadaire nominal 125 W, allumé à 100 % → puissance attendue 125 W.
	lamp := &models.Lampadaire{NominalPowerW: iptr(125), Intensite: 100}

	cases := []struct {
		name     string
		m        *models.SensorMeasurement
		wantType int
	}{
		{"sain", &models.SensorMeasurement{Courant: fptr(2.5), Tension: fptr(225), Puissance: fptr(125)}, 0},
		{"surintensité", &models.SensorMeasurement{Courant: fptr(6.0), Tension: fptr(225), Puissance: fptr(125)}, 1},
		{"surtension", &models.SensorMeasurement{Courant: fptr(2.5), Tension: fptr(240), Puissance: fptr(125)}, 2},
		{"sous-consommation", &models.SensorMeasurement{Courant: fptr(2.5), Tension: fptr(225), Puissance: fptr(80)}, 3},
		{"limite haute saine", &models.SensorMeasurement{Courant: fptr(3.9), Tension: fptr(232), Puissance: fptr(120)}, 0},
	}

	for _, c := range cases {
		got := ClassifyFault(lamp, c.m)
		if got.FaultType != c.wantType {
			t.Errorf("%s: fault_type = %d, attendu %d (%s)", c.name, got.FaultType, c.wantType, got.Label)
		}
	}
}

func TestClassifyFaultRelativeUnderpower(t *testing.T) {
	// Un lampadaire volontairement atténué à 40 % ne doit PAS être signalé en
	// sous-consommation : attendu = 125 × 0.40 = 50 W ; mesuré 48 W (> 70 % × 50 = 35).
	lamp := &models.Lampadaire{NominalPowerW: iptr(125), Intensite: 40}
	m := &models.SensorMeasurement{Courant: fptr(2.5), Tension: fptr(225), Puissance: fptr(48)}
	if got := ClassifyFault(lamp, m); got.FaultType != 0 {
		t.Errorf("dimming volontaire faussement signalé : fault_type = %d (%s)", got.FaultType, got.Label)
	}
}
