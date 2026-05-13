package controllers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/models"
	"map-interactif/internal/services"
)

// HandlePostTelemetry handles POST /api/telemetry.
func HandlePostTelemetry(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var m models.SensorMeasurement
		if err := c.BindJSON(&m); err != nil {
			RespondError(c, http.StatusBadRequest, "Payload JSON invalide.")
			return
		}

		if m.LampadaireID <= 0 {
			if m.LCUReference != "" && m.DeviceUID != "" {
				err := db.QueryRowContext(c.Request.Context(), `
					SELECT l.id FROM lampadaires l
					JOIN lcus lc ON l.lcu_id = lc.id
					WHERE lc.reference = $1 AND l.device_uid = $2
				`, m.LCUReference, m.DeviceUID).Scan(&m.LampadaireID)
				if err != nil {
					RespondError(c, http.StatusNotFound, "Lampadaire introuvable pour cette LCU et Device UID.")
					return
				}
			} else {
				RespondError(c, http.StatusBadRequest, "lampadaire_id ou (lcu_reference + device_uid) requis.")
				return
			}
		}

		if m.LCUReference != "" {
			db.ExecContext(c.Request.Context(), "UPDATE lcus SET last_seen_at = NOW() WHERE reference = $1", m.LCUReference)
		} else {
			var lcuID sql.NullInt64
			db.QueryRowContext(c.Request.Context(), "SELECT lcu_id FROM lampadaires WHERE id = $1", m.LampadaireID).Scan(&lcuID)
			if lcuID.Valid {
				db.ExecContext(c.Request.Context(), "UPDATE lcus SET last_seen_at = NOW() WHERE id = $1", lcuID.Int64)
			}
		}

		if m.Source == "" {
			m.Source = "simulation"
		}

		if m.Temperature != nil && (*m.Temperature < -40 || *m.Temperature > 120) {
			RespondError(c, http.StatusBadRequest, "Température hors plage réaliste (-40 à 120).")
			return
		}
		if m.Humidite != nil && (*m.Humidite < 0 || *m.Humidite > 100) {
			RespondError(c, http.StatusBadRequest, "Humidité invalide (0-100).")
			return
		}
		if m.Tension != nil && *m.Tension < 0 {
			RespondError(c, http.StatusBadRequest, "Tension négative invalide.")
			return
		}
		if m.Courant != nil && *m.Courant < 0 {
			RespondError(c, http.StatusBadRequest, "Courant négatif invalide.")
			return
		}
		if m.Puissance != nil && *m.Puissance < 0 {
			RespondError(c, http.StatusBadRequest, "Puissance négative invalide.")
			return
		}
		if m.Energie != nil && *m.Energie < 0 {
			RespondError(c, http.StatusBadRequest, "Énergie négative invalide.")
			return
		}

		tx, err := db.BeginTx(c.Request.Context(), nil)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur transaction.")
			return
		}
		defer tx.Rollback()

		var lmp models.Lampadaire
		err = tx.QueryRowContext(c.Request.Context(), `
			SELECT id, reference, etat, puissance FROM lampadaires WHERE id = $1 AND archived_at IS NULL FOR UPDATE
		`, m.LampadaireID).Scan(&lmp.ID, &lmp.Reference, &lmp.Etat, &lmp.Puissance)
		if err != nil {
			RespondError(c, http.StatusNotFound, "Lampadaire introuvable.")
			return
		}

		var insertedID int
		err = tx.QueryRowContext(c.Request.Context(), `
			INSERT INTO sensor_measurements
			(lampadaire_id, luminosite, presence, temperature, humidite, tension, courant, puissance, energie, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING id
		`, m.LampadaireID, m.Luminosite, m.Presence, m.Temperature, m.Humidite,
			m.Tension, m.Courant, m.Puissance, m.Energie, m.Source).Scan(&insertedID)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur insertion mesure.")
			return
		}
		m.ID = insertedID

		newEtat := lmp.Etat
		if newEtat == "offline" {
			newEtat = "online"
		}

		_, err = tx.ExecContext(c.Request.Context(), `
			UPDATE lampadaires SET last_seen_at = NOW(), etat = $1, updated_at = NOW() WHERE id = $2
		`, newEtat, m.LampadaireID)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur mise à jour lampadaire.")
			return
		}

		alerts := services.RunAlertRules(c.Request.Context(), tx, &lmp, &m)

		if err := tx.Commit(); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur commit.")
			return
		}

		RespondJSON(c, http.StatusCreated, gin.H{
			"measurement": m,
			"alerts":      alerts,
		})
	}
}

// HandleGetTelemetry handles GET /api/lampadaires/:id/telemetry.
func HandleGetTelemetry(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "Identifiant invalide.")
			return
		}

		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT id, lampadaire_id, luminosite, presence, temperature, humidite,
				tension, courant, puissance, energie, source, created_at
			FROM sensor_measurements
			WHERE lampadaire_id=$1
			ORDER BY created_at DESC LIMIT 50
		`, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur de requête.")
			return
		}
		defer rows.Close()

		measurements := []models.SensorMeasurement{}
		for rows.Next() {
			var m models.SensorMeasurement
			if err := rows.Scan(&m.ID, &m.LampadaireID, &m.Luminosite, &m.Presence,
				&m.Temperature, &m.Humidite, &m.Tension, &m.Courant,
				&m.Puissance, &m.Energie, &m.Source, &m.CreatedAt); err != nil {
				continue
			}
			measurements = append(measurements, m)
		}
		RespondJSON(c, http.StatusOK, measurements)
	}
}

// HandleGetTelemetryLatest handles GET /api/lampadaires/:id/telemetry/latest.
func HandleGetTelemetryLatest(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "Identifiant invalide.")
			return
		}

		row := db.QueryRowContext(c.Request.Context(), `
			SELECT id, lampadaire_id, luminosite, presence, temperature, humidite,
				tension, courant, puissance, energie, source, created_at
			FROM sensor_measurements
			WHERE lampadaire_id=$1
			ORDER BY created_at DESC LIMIT 1
		`, id)

		var m models.SensorMeasurement
		err = row.Scan(&m.ID, &m.LampadaireID, &m.Luminosite, &m.Presence,
			&m.Temperature, &m.Humidite, &m.Tension, &m.Courant,
			&m.Puissance, &m.Energie, &m.Source, &m.CreatedAt)
		if err != nil {
			RespondError(c, http.StatusNotFound, "Aucune mesure trouvée.")
			return
		}

		RespondJSON(c, http.StatusOK, m)
	}
}
