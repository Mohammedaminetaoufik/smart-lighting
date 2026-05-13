package main

import (
	"database/sql"
	"math/rand"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func handleSimulateTelemetry(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "Identifiant invalide.")
			return
		}

		anomaly := c.Query("anomaly") == "true"

		lamp, err := getLampadaireByID(c.Request.Context(), db, id)
		if err != nil {
			respondError(c, http.StatusNotFound, "Lampadaire introuvable.")
			return
		}

		m := generateMeasurement(lamp, anomaly)

		// Start transaction for telemetry (standard practice now)
		tx, err := db.BeginTx(c.Request.Context(), nil)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Transaction error.")
			return
		}
		defer tx.Rollback()

		var insertedID int
		err = tx.QueryRowContext(c.Request.Context(), `
			INSERT INTO sensor_measurements
			(lampadaire_id, luminosite, presence, temperature, humidite, tension, courant, puissance, energie, source)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id
		`, id, m.Luminosite, m.Presence, m.Temperature, m.Humidite,
			m.Tension, m.Courant, m.Puissance, m.Energie, m.Source).Scan(&insertedID)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur insertion.")
			return
		}
		m.ID = insertedID
		m.LampadaireID = id

		// Update status
		newEtat := lamp.Etat
		if newEtat == "offline" {
			newEtat = "online"
		}
		tx.ExecContext(c.Request.Context(), `UPDATE lampadaires SET last_seen_at=NOW(), etat=$1, updated_at=NOW() WHERE id=$2`, newEtat, id)

		// Run alert rules
		alerts := runAlertRules(c.Request.Context(), tx, lamp, &m)

		if err := tx.Commit(); err != nil {
			respondError(c, http.StatusInternalServerError, "Commit error.")
			return
		}

		respondJSON(c, http.StatusCreated, gin.H{"measurement": m, "alerts": alerts})
	}
}

func handleSimulateAll(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.QueryContext(c.Request.Context(), `SELECT id FROM lampadaires WHERE archived_at IS NULL`)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur.")
			return
		}
		defer rows.Close()

		var ids []int
		for rows.Next() {
			var id int
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}

		var measurements []SensorMeasurement
		for _, id := range ids {
			lamp, err := getLampadaireByID(c.Request.Context(), db, id)
			if err != nil {
				continue
			}
			m := generateMeasurement(lamp, false)

			tx, _ := db.BeginTx(c.Request.Context(), nil)
			var insertedID int
			tx.QueryRowContext(c.Request.Context(), `
				INSERT INTO sensor_measurements
				(lampadaire_id, luminosite, presence, temperature, humidite, tension, courant, puissance, energie, source)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id
			`, id, m.Luminosite, m.Presence, m.Temperature, m.Humidite,
				m.Tension, m.Courant, m.Puissance, m.Energie, m.Source).Scan(&insertedID)

			m.ID = insertedID
			m.LampadaireID = id
			measurements = append(measurements, m)

			newEtat := lamp.Etat
			if newEtat == "offline" {
				newEtat = "online"
			}
			tx.ExecContext(c.Request.Context(), `UPDATE lampadaires SET last_seen_at=NOW(), etat=$1, updated_at=NOW() WHERE id=$2`, newEtat, id)
			runAlertRules(c.Request.Context(), tx, lamp, &m)
			tx.Commit()
		}

		respondJSON(c, http.StatusCreated, gin.H{"count": len(measurements), "measurements": measurements})
	}
}

// SimScenario describes a named simulation scenario.
type SimScenario struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

var simScenarios = []SimScenario{
	{Name: "night_normal", Description: "Nuit calme — intensité réduite, pas de présence"},
	{Name: "pedestrian_detected", Description: "Piéton détecté — intensité 90%"},
	{Name: "vehicle_detected", Description: "Véhicule détecté — luminosité faible, intensité max"},
	{Name: "overheat", Description: "Surchauffe LED — température 78°C, alerte critique"},
	{Name: "driver_fault", Description: "Panne driver — puissance 0W"},
	{Name: "communication_lost", Description: "Perte communication — lampadaire offline"},
	{Name: "day_burner", Description: "Allumé de jour — luminosité >80 avec alerte day_burner"},
	{Name: "abnormal_power", Description: "Consommation anormale — 1.6x puissance nominale"},
}

func handleGetScenarios() gin.HandlerFunc {
	return func(c *gin.Context) {
		respondJSON(c, http.StatusOK, simScenarios)
	}
}

func handleRunScenario(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Scenario     string `json:"scenario"`
			LampadaireID int    `json:"lampadaire_id"`
		}
		if err := c.BindJSON(&body); err != nil || body.Scenario == "" || body.LampadaireID <= 0 {
			respondError(c, http.StatusBadRequest, "scenario et lampadaire_id sont obligatoires")
			return
		}
		lamp, err := getLampadaireByID(c.Request.Context(), db, body.LampadaireID)
		if err != nil {
			respondError(c, http.StatusNotFound, "Lampadaire introuvable")
			return
		}
		m, alertMsg := buildScenarioMeasurement(lamp, body.Scenario)
		tx, err := db.BeginTx(c.Request.Context(), nil)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Transaction error")
			return
		}
		defer tx.Rollback()
		var insertedID int
		if err := tx.QueryRowContext(c.Request.Context(), `
			INSERT INTO sensor_measurements
			(lampadaire_id, luminosite, presence, temperature, humidite, tension, courant, puissance, energie, source)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
			body.LampadaireID, m.Luminosite, m.Presence, m.Temperature, m.Humidite,
			m.Tension, m.Courant, m.Puissance, m.Energie, "scenario:"+body.Scenario,
		).Scan(&insertedID); err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur insertion")
			return
		}
		m.ID = insertedID
		m.LampadaireID = body.LampadaireID
		if body.Scenario == "communication_lost" {
			tx.ExecContext(c.Request.Context(), `UPDATE lampadaires SET etat='offline', updated_at=NOW() WHERE id=$1`, body.LampadaireID)
		} else {
			tx.ExecContext(c.Request.Context(), `UPDATE lampadaires SET last_seen_at=NOW(), etat='online', updated_at=NOW() WHERE id=$1`, body.LampadaireID)
		}
		runAlertRules(c.Request.Context(), tx, lamp, &m)
		if err := tx.Commit(); err != nil {
			respondError(c, http.StatusInternalServerError, "Commit error")
			return
		}
		result := gin.H{"scenario": body.Scenario, "measurement": m}
		if alertMsg != "" {
			result["alert_triggered"] = alertMsg
		}
		respondJSON(c, http.StatusCreated, result)
	}
}

type scenarioCfg struct {
	lumMin, lumMax float64
	presence       bool
	tempMin        float64
	powerFactor    float64
	alertMsg       string
}

var scenarioConfigs = map[string]scenarioCfg{
	"night_normal":       {lumMin: 5, lumMax: 15, presence: false, tempMin: 15, powerFactor: 0.30},
	"pedestrian_detected":{lumMin: 5, lumMax: 15, presence: true,  tempMin: 20, powerFactor: 0.90},
	"vehicle_detected":   {lumMin: 2, lumMax: 7,  presence: true,  tempMin: 22, powerFactor: 1.00},
	"overheat":           {lumMin: 5, lumMax: 5,  presence: false, tempMin: 78, powerFactor: 0.60, alertMsg: "Surchauffe: température 78°C"},
	"driver_fault":       {lumMin: 0, lumMax: 0,  presence: false, tempMin: 25, powerFactor: 0.00, alertMsg: "Panne driver: puissance 0W"},
	"day_burner":         {lumMin: 85, lumMax: 95, presence: false, tempMin: 30, powerFactor: 0.80, alertMsg: "Day burner: luminosité 85 mais lampadaire allumé"},
	"abnormal_power":     {lumMin: 5, lumMax: 5,  presence: false, tempMin: 28, powerFactor: 1.60, alertMsg: "Consommation anormale: 1.6x nominal"},
}

func buildScenarioMeasurement(lamp *Lampadaire, scenario string) (SensorMeasurement, string) {
	if scenario == "communication_lost" {
		return SensorMeasurement{Source: "simulation"}, "Communication perdue"
	}
	nominalPower := 100.0
	if lamp.Puissance != nil && *lamp.Puissance > 0 {
		nominalPower = float64(*lamp.Puissance)
	}
	cfg, ok := scenarioConfigs[scenario]
	if !ok {
		return generateMeasurement(lamp, false), ""
	}
	tension := 225.0 + rand.Float64()*10
	puiss := nominalPower * cfg.powerFactor
	lum := cfg.lumMin + rand.Float64()*(cfg.lumMax-cfg.lumMin+1)
	return SensorMeasurement{
		Luminosite:  ptrFloat64(lum),
		Presence:    ptrBool(cfg.presence),
		Temperature: ptrFloat64(cfg.tempMin + rand.Float64()*3),
		Humidite:    ptrFloat64(45 + rand.Float64()*20),
		Tension:     ptrFloat64(tension),
		Courant:     ptrFloat64(puiss / tension),
		Puissance:   ptrFloat64(puiss),
		Energie:     ptrFloat64(puiss * 0.005),
		Source:      "simulation",
	}, cfg.alertMsg
}

func generateMeasurement(lamp *Lampadaire, anomaly bool) SensorMeasurement {
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
		pres = rand.Intn(10) > 7 // 30% chance at night
	} else {
		pres = rand.Intn(10) > 3 // 70% chance during day
	}

	temp := 15 + rand.Float64()*15 // Normal 15-30
	if anomaly {
		temp = 80 + rand.Float64()*20 // Critical
	}

	hum := 40 + rand.Float64()*30
	tension := 225 + rand.Float64()*10

	puiss := nominalPower * intensityFactor * (0.95 + rand.Float64()*0.1)
	if anomaly && rand.Intn(2) == 0 {
		puiss = nominalPower * 1.6 // Power anomaly
	}

	courant := puiss / tension
	energie := puiss * 0.005

	return SensorMeasurement{
		Luminosite:  ptrFloat64(lum),
		Presence:    ptrBool(pres),
		Temperature: ptrFloat64(temp),
		Humidite:    ptrFloat64(hum),
		Tension:     ptrFloat64(tension),
		Courant:     ptrFloat64(courant),
		Puissance:   ptrFloat64(puiss),
		Energie:     ptrFloat64(energie),
		Source:      "simulation",
	}
}
