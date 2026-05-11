package main

import (
	"time"
)

type CalculatorResult struct {
	RecommendedIntensity int
	Reason               string
	Confidence           float64
	RuleName             string
}

// calculateRecommendedIntensity is the core logic of the intelligent calculator.
func calculateRecommendedIntensity(lamp *Lampadaire, m *SensorMeasurement, now time.Time) CalculatorResult {
	hour := now.Hour()

	// 1. Maintenance
	if lamp.Etat == "maintenance" {
		return CalculatorResult{
			RecommendedIntensity: 0,
			Reason:               "Lampadaire en maintenance",
			Confidence:           1.0,
			RuleName:             "maintenance_mode",
		}
	}

	// Logic with telemetry
	if m != nil {
		// 2. High Temperature Protection
		if m.Temperature != nil && *m.Temperature > 75 {
			return CalculatorResult{
				RecommendedIntensity: minInt(lamp.Intensite, 50),
				Reason:               "Réduction pour protection thermique",
				Confidence:           0.95,
				RuleName:             "high_temperature_protection",
			}
		}

		// 3. Presence + Low Luminosity
		if m.Presence != nil && *m.Presence && m.Luminosite != nil && *m.Luminosite < 30 {
			return CalculatorResult{
				RecommendedIntensity: 90,
				Reason:               "Présence détectée et faible luminosité",
				Confidence:           0.9,
				RuleName:             "presence_low_luminosity",
			}
		}

		// 4. Night Low Activity (No presence)
		if m.Presence != nil && !*m.Presence && hour >= 0 && hour < 5 {
			return CalculatorResult{
				RecommendedIntensity: 30,
				Reason:               "Faible activité nocturne",
				Confidence:           0.85,
				RuleName:             "night_low_activity",
			}
		}

		// 5. Low Luminosity without Presence
		if m.Presence != nil && !*m.Presence && m.Luminosite != nil && *m.Luminosite < 30 {
			return CalculatorResult{
				RecommendedIntensity: 50,
				Reason:               "Faible luminosité sans présence",
				Confidence:           0.8,
				RuleName:             "no_presence_low_luminosity",
			}
		}

		// 6. High Ambient Luminosity
		if m.Luminosite != nil && *m.Luminosite > 70 {
			return CalculatorResult{
				RecommendedIntensity: 20,
				Reason:               "Luminosité ambiante suffisante",
				Confidence:           0.85,
				RuleName:             "high_ambient_luminosity",
			}
		}
	}

	// 7. Default Optimized
	return CalculatorResult{
		RecommendedIntensity: 60,
		Reason:               "Niveau standard optimisé",
		Confidence:           0.7,
		RuleName:             "standard_optimized",
	}
}
