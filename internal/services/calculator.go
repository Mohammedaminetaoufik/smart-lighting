package services

import (
	"time"

	"map-interactif/internal/models"
	"map-interactif/internal/utils"
)

// CalculatorResult holds the result from the intelligent calculator.
type CalculatorResult struct {
	RecommendedIntensity int
	Reason               string
	Confidence           float64
	RuleName             string
}

// CalculateRecommendedIntensity is the core logic of the intelligent calculator.
func CalculateRecommendedIntensity(lamp *models.Lampadaire, m *models.SensorMeasurement, now time.Time) CalculatorResult {
	// Astronomical context: compute real dusk/dawn for this lamp's position when
	// coordinates are available. This drives dusk-to-dawn switching instead of a
	// hardcoded clock hour — the signature of proper public-lighting control.
	isNight, deepNight := astronomicalNight(lamp, now)

	// 1. Maintenance
	if lamp.Etat == "maintenance" {
		return CalculatorResult{
			RecommendedIntensity: 0,
			Reason:               "Lampadaire en maintenance",
			Confidence:           1.0,
			RuleName:             "maintenance_mode",
		}
	}

	// 0. Daytime (after sunrise, before sunset): lamp should be off.
	if !isNight {
		return CalculatorResult{
			RecommendedIntensity: 0,
			Reason:               "Jour — extinction automatique (calendrier astronomique)",
			Confidence:           0.95,
			RuleName:             "astronomical_daytime",
		}
	}

	// Logic with telemetry
	if m != nil {
		// 2. High Temperature Protection (priorité sécurité, avant tout profil)
		if m.Temperature != nil && *m.Temperature > 75 {
			return CalculatorResult{
				RecommendedIntensity: utils.MinInt(lamp.Intensite, 50),
				Reason:               "Réduction pour protection thermique",
				Confidence:           0.95,
				RuleName:             "high_temperature_protection",
			}
		}

		// 2b. Profil appris du dataset (data-driven) — prioritaire sur les règles
		// codées en dur quand une référence existe pour ces conditions.
		if isNight && m.Luminosite != nil && m.Presence != nil {
			if b, ok := LookupRecommendedBrightness(now.Hour(), *m.Luminosite, *m.Presence); ok {
				return CalculatorResult{
					RecommendedIntensity: b,
					Reason:               "Intensité recommandée par le profil appris du dataset",
					Confidence:           0.9,
					RuleName:             "data_driven_reference",
				}
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

		// 4. Deep-night low activity (no presence) — based on real solar midnight
		// window, not a fixed clock hour.
		if m.Presence != nil && !*m.Presence && deepNight {
			return CalculatorResult{
				RecommendedIntensity: 30,
				Reason:               "Faible activité en cœur de nuit",
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

// astronomicalNight determines whether it is night (lamp should be lit) and
// whether it is deep night (core hours, low activity) for a lamp's position.
// When coordinates are available it uses real sunrise/sunset; otherwise it falls
// back to a fixed-hour heuristic.
func astronomicalNight(lamp *models.Lampadaire, now time.Time) (isNight, deepNight bool) {
	if lamp == nil || lamp.Latitude == nil || lamp.Longitude == nil ||
		(*lamp.Latitude == 0 && *lamp.Longitude == 0) {
		h := now.Hour()
		return h < 7 || h >= 19, h >= 23 || h < 5
	}

	sun := SunriseSunset(*lamp.Latitude, *lamp.Longitude, now, now.Location())
	if sun.Sunrise.IsZero() || sun.Sunset.IsZero() {
		h := now.Hour()
		return h < 7 || h >= 19, h >= 23 || h < 5
	}

	isNight = now.Before(sun.Sunrise) || now.After(sun.Sunset)
	// Deep night = at least 2h after dusk and at least 2h before dawn.
	deepNight = now.After(sun.Sunset.Add(2*time.Hour)) || now.Before(sun.Sunrise.Add(-2*time.Hour))
	return isNight, deepNight
}
