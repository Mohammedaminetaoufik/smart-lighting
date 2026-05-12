package main

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

func handleGetInterventions(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.Query(`
			SELECT i.id, i.alert_id, i.lampadaire_id, i.assigned_to, i.title, i.description, i.priority, i.status, i.created_at, i.updated_at, i.closed_at
			FROM interventions i
			ORDER BY i.created_at DESC
		`)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()
		var items []Intervention
		for rows.Next() {
			var i Intervention
			if err := rows.Scan(&i.ID, &i.AlertID, &i.LampadaireID, &i.AssignedTo, &i.Title, &i.Description, &i.Priority, &i.Status, &i.CreatedAt, &i.UpdatedAt, &i.ClosedAt); err != nil {
				continue
			}
			items = append(items, i)
		}
		respondJSON(c, http.StatusOK, items)
	}
}

func handleCreateIntervention(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var i Intervention
		if err := c.BindJSON(&i); err != nil {
			respondError(c, http.StatusBadRequest, "Invalid JSON")
			return
		}
		err := db.QueryRow(`
			INSERT INTO interventions (alert_id, lampadaire_id, assigned_to, title, description, priority, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, created_at, updated_at
		`, i.AlertID, i.LampadaireID, i.AssignedTo, i.Title, i.Description, i.Priority, i.Status).Scan(&i.ID, &i.CreatedAt, &i.UpdatedAt)

		if err != nil {
			respondError(c, http.StatusInternalServerError, "Database error: "+err.Error())
			return
		}
		respondJSON(c, http.StatusCreated, i)
	}
}

func handleCreateInterventionFromAlert(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		alertID, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID alerte invalide")
			return
		}

		var payload struct {
			AssignedTo  *int   `json:"assigned_to"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Priority    string `json:"priority"`
		}
		if err := c.BindJSON(&payload); err != nil {
			respondError(c, http.StatusBadRequest, "Invalid JSON")
			return
		}

		var lampID int
		if err := db.QueryRow("SELECT lampadaire_id FROM alerts WHERE id = $1", alertID).Scan(&lampID); err != nil {
			respondError(c, http.StatusNotFound, "Alerte introuvable")
			return
		}

		var i Intervention
		err = db.QueryRow(`
			INSERT INTO interventions (alert_id, lampadaire_id, assigned_to, title, description, priority, status)
			VALUES ($1, $2, $3, $4, $5, $6, 'open')
			RETURNING id, alert_id, lampadaire_id, assigned_to, title, description, priority, status, created_at, updated_at
		`, alertID, lampID, payload.AssignedTo, payload.Title, payload.Description, payload.Priority).
			Scan(&i.ID, &i.AlertID, &i.LampadaireID, &i.AssignedTo, &i.Title, &i.Description, &i.Priority, &i.Status, &i.CreatedAt, &i.UpdatedAt)

		if err != nil {
			respondError(c, http.StatusInternalServerError, "Database error")
			return
		}

		if _, err := db.Exec("UPDATE alerts SET status = 'in_progress' WHERE id = $1", alertID); err != nil {
			log.Printf("handleCreateInterventionFromAlert: failed to update alert %d status: %v", alertID, err)
		}

		respondJSON(c, http.StatusCreated, i)
	}
}

func handleUpdateInterventionStatus(db *sql.DB, status string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		if _, err := db.Exec("UPDATE interventions SET status = $1, updated_at = NOW() WHERE id = $2", status, id); err != nil {
			respondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		respondJSON(c, http.StatusOK, gin.H{"status": "success", "new_status": status})
	}
}

func handleCloseIntervention(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}

		var alertID *int
		if err := db.QueryRow("UPDATE interventions SET status = 'closed', closed_at = NOW(), updated_at = NOW() WHERE id = $1 RETURNING alert_id", id).Scan(&alertID); err != nil {
			respondError(c, http.StatusNotFound, "Intervention introuvable")
			return
		}

		if alertID != nil {
			if _, err := db.Exec("UPDATE alerts SET status = 'resolved', resolved_at = NOW() WHERE id = $1", *alertID); err != nil {
				log.Printf("handleCloseIntervention: failed to resolve alert %d: %v", *alertID, err)
			}
		}

		respondJSON(c, http.StatusOK, gin.H{"status": "closed"})
	}
}
