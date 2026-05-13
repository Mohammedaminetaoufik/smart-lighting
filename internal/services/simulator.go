package services

import (
	"math/rand"
	"time"

	"map-interactif/internal/models"
	"map-interactif/internal/utils"
)

// SimScenario describes a named simulation scenario.
type SimScenario struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// SimScenarios is the list of available simulation scenarios.
var SimScenarios = []SimScenario{
	{Name: "night_normal", Description: "Nuit calme — intensité réduite, pas de présence"},
	{Name: "pedestrian_detected", Description: "Piéton détecté — intensité 90%"},
	{Name: "vehicle_detected", Description: "Véhicule détecté — luminosité faible, intensité max"},
	{Name: "overheat", Description: "Surchauffe LED — température 78°C, alerte critique"},
	{Name: "driver_fault", Description: "Panne driver — puissance 0W"},
	{Name: "communication_lost", Description: "Perte communication — lampadaire offline"},
	{Name: "day_burner", Description: "Allumé de jour — luminosité >80 avec alerte day_burner"},
	{Name: "abnormal_power", Description: "Consommation anormale — 1.6x puissance nominale"},
}

type scenarioCfg struct {
	lumMin, lumMax float64
	presence       bool
	tempMin        float64
	powerFactor    float64
	alertMsg       string
}

// ScenarioConfigs maps scenario names to their configs.
var ScenarioConfigs = map[string]scenarioCfg{
	"night_normal":        {lumMin: 5, lumMax: 15, presence: false, tempMin: 15, powerFactor: 0.30},
	"pedestrian_detected": {lumMin: 5, lumMax: 15, presence: true, tempMin: 20, powerFactor: 0.90},
	"vehicle_detected":    {lumMin: 2, lumMax: 7, presence: true, tempMin: 22, powerFactor: 1.00},
	"overheat":            {lumMin: 5, lumMax: 5, presence: false, tempMin: 78, powerFactor: 0.60, alertMsg: "Surchauffe: température 78°C"},
	"driver_fault":        {lumMin: 0, lumMax: 0, presence: false, tempMin: 25, powerFactor: 0.00, alertMsg: "Panne driver: puissance 0W"},
	"day_burner":          {lumMin: 85, lumMax: 95, presence: false, tempMin: 30, powerFactor: 0.80, alertMsg: "Day burner: luminosité 85 mais lampadaire allumé"},
	"abnormal_power":      {lumMin: 5, lumMax: 5, presence: false, tempMin: 28, powerFactor: 1.60, alertMsg: "Consommation anormale: 1.6x nominal"},
}

// BuildScenarioMeasurement builds a measurement for a specific scenario.
func BuildScenarioMeasurement(lamp *models.Lampadaire, scenario string) (models.SensorMeasurement, string) {
	if scenario == "communication_lost" {
		return models.SensorMeasurement{Source: "simulation"}, "Communication perdue"
	}
	nominalPower := 100.0
	if lamp.Puissance != nil && *lamp.Puissance > 0 {
		nominalPower = float64(*lamp.Puissance)
	}
	cfg, ok := ScenarioConfigs[scenario]
	if !ok {
		return GenerateMeasurement(lamp, false), ""
	}
	tension := 225.0 + rand.Float64()*10
	puiss := nominalPower * cfg.powerFactor
	lum := cfg.lumMin + rand.Float64()*(cfg.lumMax-cfg.lumMin+1)
	return models.SensorMeasurement{
		Luminosite:  utils.PtrFloat64(lum),
		Presence:    utils.PtrBool(cfg.presence),
		Temperature: utils.PtrFloat64(cfg.tempMin + rand.Float64()*3),
		Humidite:    utils.PtrFloat64(45 + rand.Float64()*20),
		Tension:     utils.PtrFloat64(tension),
		Courant:     utils.PtrFloat64(puiss / tension),
		Puissance:   utils.PtrFloat64(puiss),
		Energie:     utils.PtrFloat64(puiss * 0.005),
		Source:      "simulation",
	}, cfg.alertMsg
}

// GenerateMeasurement generates a realistic telemetry measurement.
func GenerateMeasurement(lamp *models.Lampadaire, anomaly bool) models.SensorMeasurement {
	nominalPower := 100.0
	if lamp.Puissance != nil && *lamp.Puissance > 0 {
		nominalPower = float64(*lamp.Puissance)
	}
	intensityFactor := float64(lamp.Intensite) / 100.0
	if intensityFactor < 0.05 {
		intensityFactor = 0.05
	}

	now := time.Now()
	hour := now.Hour()
	isNight := hour < 6 || hour > 19

	lum := 10.0 + rand.Float64()*20
	if !isNight {
		lum = 70.0 + rand.Float64()*30
	}

	pres := false
	if isNight {
		pres = rand.Intn(10) > 7
	} else {
		pres = rand.Intn(10) > 3
	}

	temp := 15 + rand.Float64()*15
	if anomaly {
		temp = 80 + rand.Float64()*20
	}

	hum := 40 + rand.Float64()*30
	tension := 225 + rand.Float64()*10

	puiss := nominalPower * intensityFactor * (0.95 + rand.Float64()*0.1)
	if anomaly && rand.Intn(2) == 0 {
		puiss = nominalPower * 1.6
	}

	courant := puiss / tension
	energie := puiss * 0.005

	return models.SensorMeasurement{
		Luminosite:  utils.PtrFloat64(lum),
		Presence:    utils.PtrBool(pres),
		Temperature: utils.PtrFloat64(temp),
		Humidite:    utils.PtrFloat64(hum),
		Tension:     utils.PtrFloat64(tension),
		Courant:     utils.PtrFloat64(courant),
		Puissance:   utils.PtrFloat64(puiss),
		Energie:     utils.PtrFloat64(energie),
		Source:      "simulation",
	}
}
