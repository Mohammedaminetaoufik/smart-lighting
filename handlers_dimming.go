package main

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

func handlePostDimming(db *sql.DB, adapter LCUAdapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "Identifiant invalide.")
			return
		}

		var body struct {
			NewIntensity int    `json:"new_intensity"`
			Source       string `json:"source"`
			Reason       string `json:"reason"`
		}
		if err := c.BindJSON(&body); err != nil {
			respondError(c, http.StatusBadRequest, "Payload JSON invalide.")
			return
		}

		if body.NewIntensity < 0 || body.NewIntensity > 100 {
			respondError(c, http.StatusBadRequest, "L'intensité doit être comprise entre 0 et 100.")
			return
		}

		if body.Source == "" {
			body.Source = "admin"
		}

		// Begin Transaction
		tx, err := db.BeginTx(c.Request.Context(), nil)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors du démarrage de la transaction.")
			return
		}
		defer tx.Rollback()

		// Get current lampadaire (within transaction to be safe)
		var oldIntensity int
		var lcuID sql.NullInt64
		var deviceUID sql.NullString
		err = tx.QueryRowContext(c.Request.Context(), `
			SELECT intensite, lcu_id, device_uid FROM lampadaires WHERE id = $1 AND archived_at IS NULL FOR UPDATE
		`, id).Scan(&oldIntensity, &lcuID, &deviceUID)
		if err != nil {
			respondError(c, http.StatusNotFound, "Lampadaire introuvable ou archivé.")
			return
		}

		// Insert dimming command
		var cmdID int
		err = tx.QueryRowContext(c.Request.Context(), `
			INSERT INTO dimming_commands (lampadaire_id, source, old_intensity, new_intensity, reason, status, applied_at)
			VALUES ($1, $2, $3, $4, $5, 'pending', NOW())
			RETURNING id
		`, id, body.Source, oldIntensity, body.NewIntensity, nullString(body.Reason)).Scan(&cmdID)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors de l'enregistrement de la commande.")
			return
		}

		// Apply via LCU if associated
		if lcuID.Valid && deviceUID.Valid && deviceUID.String != "" {
			lcu, err := getLCUByID(c.Request.Context(), db, int(lcuID.Int64))
			if err == nil {
				err = adapter.ApplyDimming(c.Request.Context(), lcu, deviceUID.String, body.NewIntensity)
				if err != nil {
					tx.ExecContext(c.Request.Context(), "UPDATE dimming_commands SET status = 'failed' WHERE id = $1", cmdID)
					createAlertIfNotExists(c.Request.Context(), tx, id, "commande_non_appliquee", "critical", "Échec LCU: "+err.Error())
					tx.Commit()
					respondError(c, http.StatusServiceUnavailable, "Échec envoi à la LCU")
					return
				}
			}
		}

		// Apply dimming: update lampadaire and command status
		_, err = tx.ExecContext(c.Request.Context(), `
			UPDATE lampadaires SET intensite = $1, last_command_at = NOW(), updated_at = NOW() WHERE id = $2
		`, body.NewIntensity, id)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors de l'application de l'intensité.")
			return
		}

		tx.ExecContext(c.Request.Context(), "UPDATE dimming_commands SET status = 'applied' WHERE id = $1", cmdID)

		// Commit
		if err := tx.Commit(); err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors de la validation de la transaction.")
			return
		}

		respondJSON(c, http.StatusOK, gin.H{
			"status": "success",
			"command": gin.H{
				"id":            cmdID,
				"old_intensity": oldIntensity,
				"new_intensity": body.NewIntensity,
				"source":        body.Source,
				"reason":        body.Reason,
				"status":        "applied",
			},
		})
	}
}

func handleGetDimmingHistory(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "Identifiant invalide.")
			return
		}

		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT id, lampadaire_id, source, old_intensity, new_intensity, reason, status, created_at, applied_at
			FROM dimming_commands
			WHERE lampadaire_id = $1
			ORDER BY created_at DESC
			LIMIT 50
		`, id)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur de requête.")
			return
		}
		defer rows.Close()

		commands := []DimmingCommand{}
		for rows.Next() {
			var cmd DimmingCommand
			var oldInt sql.NullInt64
			var reason sql.NullString
			var applied sql.NullTime
			if err := rows.Scan(&cmd.ID, &cmd.LampadaireID, &cmd.Source, &oldInt,
				&cmd.NewIntensity, &reason, &cmd.Status, &cmd.CreatedAt, &applied); err != nil {
				continue
			}
			if oldInt.Valid {
				v := int(oldInt.Int64)
				cmd.OldIntensity = &v
			}
			if reason.Valid {
				cmd.Reason = reason.String
			}
			if applied.Valid {
				cmd.AppliedAt = &applied.Time
			}
			commands = append(commands, cmd)
		}

		respondJSON(c, http.StatusOK, commands)
	}
}
