package controllers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/repository"
	"map-interactif/internal/services"
)

// HandleSimulateTelemetry handles POST /api/simulator/telemetry/:id.
func HandleSimulateTelemetry(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "Identifiant invalide.")
			return
		}

		anomaly := c.Query("anomaly") == "true"

		lamp, err := repository.GetLampadaireByID(c.Request.Context(), db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "Lampadaire introuvable.")
			return
		}

		m := services.GenerateMeasurement(lamp, anomaly)

		tx, err := db.BeginTx(c.Request.Context(), nil)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Transaction error.")
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
			RespondError(c, http.StatusInternalServerError, "Erreur insertion.")
			return
		}
		m.ID = insertedID
		m.LampadaireID = id

		newEtat := lamp.Etat
		if newEtat == "offline" {
			newEtat = "online"
		}
		tx.ExecContext(c.Request.Context(), `UPDATE lampadaires SET last_seen_at=NOW(), etat=$1, updated_at=NOW() WHERE id=$2`, newEtat, id)
		tx.ExecContext(c.Request.Context(), `UPDATE lampadaires SET controller_status='ok', controller_last_seen_at=NOW() WHERE id=$1`, id)

		alerts := services.RunAlertRules(c.Request.Context(), tx, lamp, &m)

		if err := tx.Commit(); err != nil {
			RespondError(c, http.StatusInternalServerError, "Commit error.")
			return
		}

		RespondJSON(c, http.StatusCreated, gin.H{"measurement": m, "alerts": alerts})
	}
}

// HandleSimulateAll handles POST /api/simulator/telemetry/all.
func HandleSimulateAll(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.QueryContext(c.Request.Context(), `SELECT id FROM lampadaires WHERE archived_at IS NULL`)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur.")
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

		var measurements []interface{}
		for _, id := range ids {
			lamp, err := repository.GetLampadaireByID(c.Request.Context(), db, id)
			if err != nil {
				continue
			}
			m := services.GenerateMeasurement(lamp, false)

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
			tx.ExecContext(c.Request.Context(), `UPDATE lampadaires SET controller_status='ok', controller_last_seen_at=NOW() WHERE id=$1`, id)
			services.RunAlertRules(c.Request.Context(), tx, lamp, &m)
			tx.Commit()
		}

		RespondJSON(c, http.StatusCreated, gin.H{"count": len(measurements), "measurements": measurements})
	}
}

// HandleGetScenarios handles GET /api/simulator/scenarios.
func HandleGetScenarios() gin.HandlerFunc {
	return func(c *gin.Context) {
		RespondJSON(c, http.StatusOK, services.SimScenarios)
	}
}

// HandleRunScenario handles POST /api/simulator/scenario.
func HandleRunScenario(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Scenario     string `json:"scenario"`
			LampadaireID int    `json:"lampadaire_id"`
		}
		if err := c.BindJSON(&body); err != nil || body.Scenario == "" || body.LampadaireID <= 0 {
			RespondError(c, http.StatusBadRequest, "scenario et lampadaire_id sont obligatoires")
			return
		}
		lamp, err := repository.GetLampadaireByID(c.Request.Context(), db, body.LampadaireID)
		if err != nil {
			RespondError(c, http.StatusNotFound, "Lampadaire introuvable")
			return
		}
		m, alertMsg := services.BuildScenarioMeasurement(lamp, body.Scenario)
		tx, err := db.BeginTx(c.Request.Context(), nil)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Transaction error")
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
			RespondError(c, http.StatusInternalServerError, "Erreur insertion")
			return
		}
		m.ID = insertedID
		m.LampadaireID = body.LampadaireID
		if body.Scenario == "communication_lost" {
			tx.ExecContext(c.Request.Context(), `UPDATE lampadaires SET etat='offline', updated_at=NOW() WHERE id=$1`, body.LampadaireID)
			tx.ExecContext(c.Request.Context(), `UPDATE lampadaires SET controller_status='lost', controller_last_seen_at=NOW() WHERE id=$1`, body.LampadaireID)
		} else {
			tx.ExecContext(c.Request.Context(), `UPDATE lampadaires SET last_seen_at=NOW(), etat='online', updated_at=NOW() WHERE id=$1`, body.LampadaireID)
			tx.ExecContext(c.Request.Context(), `UPDATE lampadaires SET controller_status='ok', controller_last_seen_at=NOW() WHERE id=$1`, body.LampadaireID)
		}
		services.RunAlertRules(c.Request.Context(), tx, lamp, &m)
		if err := tx.Commit(); err != nil {
			RespondError(c, http.StatusInternalServerError, "Commit error")
			return
		}
		result := gin.H{"scenario": body.Scenario, "measurement": m}
		if alertMsg != "" {
			result["alert_triggered"] = alertMsg
		}
		RespondJSON(c, http.StatusCreated, result)
	}
}
