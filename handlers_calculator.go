package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func runIntelligentCalculator(ctx context.Context, db *sql.DB, adapter LCUAdapter, lampadaireID int, apply bool) (*CalculatorDecision, error) {
	lamp, err := getLampadaireByID(ctx, db, lampadaireID)
	if err != nil {
		return nil, err
	}

	var m *SensorMeasurement
	var sm SensorMeasurement
	err = db.QueryRowContext(ctx, `
		SELECT id, lampadaire_id, luminosite, presence, temperature, humidite,
			tension, courant, puissance, energie, source, created_at
		FROM sensor_measurements WHERE lampadaire_id=$1 ORDER BY created_at DESC LIMIT 1
	`, lampadaireID).Scan(&sm.ID, &sm.LampadaireID, &sm.Luminosite, &sm.Presence,
		&sm.Temperature, &sm.Humidite, &sm.Tension, &sm.Courant,
		&sm.Puissance, &sm.Energie, &sm.Source, &sm.CreatedAt)
	if err == nil {
		m = &sm
	}

	res := calculateRecommendedIntensity(lamp, m, time.Now())

	// Begin Transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	d := CalculatorDecision{
		LampadaireID:         lampadaireID,
		RecommendedIntensity: res.RecommendedIntensity,
		DecisionReason:       res.Reason,
		Confidence:           res.Confidence,
		RuleName:             res.RuleName,
	}

	var decisionID int
	err = tx.QueryRowContext(ctx, `
		INSERT INTO calculator_decisions (lampadaire_id, recommended_intensity, decision_reason, confidence, applied, rule_name)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, created_at
	`, lampadaireID, d.RecommendedIntensity, d.DecisionReason, d.Confidence, false, d.RuleName).Scan(&decisionID, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	d.ID = decisionID

	if apply {
		// Insert dimming command
		var cmdID int
		err = tx.QueryRowContext(ctx, `
			INSERT INTO dimming_commands (lampadaire_id, source, old_intensity, new_intensity, reason, status, applied_at)
			VALUES ($1,'calculateur_intelligent',$2,$3,$4,'pending',NOW())
			RETURNING id
		`, lampadaireID, lamp.Intensite, d.RecommendedIntensity, d.DecisionReason).Scan(&cmdID)
		if err != nil {
			return nil, err
		}

		// Hardware Apply via LCU if associated
		if lamp.LCUID != nil && lamp.DeviceUID != "" {
			lcu, err := getLCUByID(ctx, db, *lamp.LCUID)
			if err == nil {
				err = adapter.ApplyDimming(ctx, lcu, lamp.DeviceUID, d.RecommendedIntensity, d.DecisionReason, "calculateur_intelligent")
				if err != nil {
					// Mark command as failed and create alert
					tx.ExecContext(ctx, "UPDATE dimming_commands SET status = 'failed' WHERE id = $1", cmdID)
					createAlertIfNotExists(ctx, tx, lampadaireID, "commande_calculateur_echec", "critical", "Calculateur: Échec envoi LCU: "+err.Error())
					tx.Commit()
					return &d, fmt.Errorf("échec envoi à la LCU: %w", err)
				}
			}
		}

		// Update lampadaire
		_, err = tx.ExecContext(ctx, `
			UPDATE lampadaires SET intensite=$1, last_command_at=NOW(), updated_at=NOW() WHERE id=$2
		`, d.RecommendedIntensity, lampadaireID)
		if err != nil {
			return nil, err
		}

		// Mark decision as applied and command as applied
		tx.ExecContext(ctx, "UPDATE dimming_commands SET status = 'applied' WHERE id = $1", cmdID)
		_, err = tx.ExecContext(ctx, `UPDATE calculator_decisions SET applied=true WHERE id=$1`, d.ID)
		if err != nil {
			return nil, err
		}
		d.Applied = true
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &d, nil
}

func handleRunCalculator(db *sql.DB, adapter LCUAdapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "Identifiant invalide.")
			return
		}
		var body struct {
			Apply bool `json:"apply"`
		}
		c.BindJSON(&body)
		decision, err := runIntelligentCalculator(c.Request.Context(), db, adapter, id, body.Apply)
		if err != nil {
			log.Printf("calculator error lamp=%d: %v", id, err)
			respondError(c, http.StatusInternalServerError, "Erreur calculateur: "+err.Error())
			return
		}
		respondJSON(c, http.StatusOK, decision)
	}
}

func handleRunCalculatorAll(db *sql.DB, adapter LCUAdapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Apply bool `json:"apply"`
		}
		c.BindJSON(&body)
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
		decisions := []CalculatorDecision{}
		for _, id := range ids {
			d, err := runIntelligentCalculator(c.Request.Context(), db, adapter, id, body.Apply)
			if err == nil && d != nil {
				decisions = append(decisions, *d)
			}
		}
		respondJSON(c, http.StatusOK, gin.H{"count": len(decisions), "decisions": decisions})
	}
}

func handleGetDecisions(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "Identifiant invalide.")
			return
		}
		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT id, lampadaire_id, recommended_intensity, decision_reason, confidence, applied, rule_name, created_at
			FROM calculator_decisions WHERE lampadaire_id=$1 ORDER BY created_at DESC LIMIT 20`, id)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur.")
			return
		}
		defer rows.Close()
		decisions := []CalculatorDecision{}
		for rows.Next() {
			var d CalculatorDecision
			var ruleName sql.NullString
			if err := rows.Scan(&d.ID, &d.LampadaireID, &d.RecommendedIntensity, &d.DecisionReason, &d.Confidence, &d.Applied, &ruleName, &d.CreatedAt); err == nil {
				if ruleName.Valid {
					d.RuleName = ruleName.String
				}
				decisions = append(decisions, d)
			}
		}
		respondJSON(c, http.StatusOK, decisions)
	}
}
