package main

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

func handleAckAlert(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		_, err = db.Exec(`
			UPDATE alerts SET status='acknowledged', acknowledged_at=NOW()
			WHERE id=$1 AND status='open'`, id)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		respondJSON(c, http.StatusOK, gin.H{"status": "acknowledged", "alert_id": id})
	}
}

func handleCloseAlert(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		_, err = db.Exec(`
			UPDATE alerts SET status='closed', closed_at=NOW()
			WHERE id=$1 AND status NOT IN ('closed')`, id)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		respondJSON(c, http.StatusOK, gin.H{"status": "closed", "alert_id": id})
	}
}
