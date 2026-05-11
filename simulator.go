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
