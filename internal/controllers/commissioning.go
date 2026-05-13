package controllers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/repository"
)

// commissioning step constants
const (
	stepDiscovered   = 0
	stepLocated      = 1
	stepConfigured   = 2
	stepTested       = 3
	stepCommissioned = 4
	stepFailed       = -1
)

var commissioningStatusByStep = map[int]string{
	stepDiscovered:   "discovered",
	stepLocated:      "located",
	stepConfigured:   "configured",
	stepTested:       "tested",
	stepCommissioned: "commissioned",
	stepFailed:       "failed",
}

// HandleAdvanceCommissioning handles POST /api/commissioning/:id/advance.
func HandleAdvanceCommissioning(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var currentStep int
		if err := db.QueryRow(`SELECT commissioning_step FROM lampadaires WHERE id=$1`, id).Scan(&currentStep); err != nil {
			RespondError(c, http.StatusNotFound, "Lampadaire introuvable")
			return
		}
		if currentStep >= stepCommissioned {
			RespondError(c, http.StatusBadRequest, "Déjà commissioning complété")
			return
		}
		newStep := currentStep + 1
		newStatus := commissioningStatusByStep[newStep]
		if err := updateCommissioningStep(db, id, newStep, newStatus); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur mise à jour")
			return
		}
		RespondJSON(c, http.StatusOK, gin.H{"step": newStep, "status": newStatus, "lampadaire_id": id})
	}
}

// HandleTestCommCommissioning handles POST /api/commissioning/:id/test-comm.
func HandleTestCommCommissioning(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var body struct {
			Result string `json:"result"` // "ok" or "failed"
		}
		_ = c.BindJSON(&body)
		result := body.Result
		if result != "ok" && result != "failed" {
			result = "ok"
		}
		if _, err := db.Exec(`UPDATE lampadaires SET test_comm_status=$1, updated_at=NOW() WHERE id=$2`, result, id); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur mise à jour")
			return
		}
		RespondJSON(c, http.StatusOK, gin.H{"test_comm_status": result, "lampadaire_id": id})
	}
}

// HandleTestDimmingCommissioning handles POST /api/commissioning/:id/test-dimming.
func HandleTestDimmingCommissioning(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var body struct {
			Result string `json:"result"` // "ok" or "failed"
		}
		_ = c.BindJSON(&body)
		result := body.Result
		if result != "ok" && result != "failed" {
			result = "ok"
		}
		if _, err := db.Exec(`UPDATE lampadaires SET test_dimming_status=$1, updated_at=NOW() WHERE id=$2`, result, id); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur mise à jour")
			return
		}
		RespondJSON(c, http.StatusOK, gin.H{"test_dimming_status": result, "lampadaire_id": id})
	}
}

// HandleValidateCommissioning handles POST /api/commissioning/:id/validate.
func HandleValidateCommissioning(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		now := time.Now()
		if _, err := db.Exec(`
			UPDATE lampadaires
			SET commissioning_step=$1, commissioning_status='commissioned',
			    commissioned_at=$2, updated_at=$2
			WHERE id=$3`, stepCommissioned, now, id); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur mise à jour")
			return
		}
		RespondJSON(c, http.StatusOK, gin.H{"status": "commissioned", "lampadaire_id": id, "commissioned_at": now})
	}
}

// HandleFailCommissioning handles POST /api/commissioning/:id/fail.
func HandleFailCommissioning(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var body struct {
			Notes string `json:"notes"`
		}
		_ = c.BindJSON(&body)
		if _, err := db.Exec(`
			UPDATE lampadaires
			SET commissioning_step=$1, commissioning_status='failed',
			    commissioning_notes=$2, updated_at=NOW()
			WHERE id=$3`, stepFailed, body.Notes, id); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur mise à jour")
			return
		}
		repository.CreateAlertIfNotExists(context.Background(), db, id, "commissioning_failed", "warning",
			"Échec du commissioning pour le lampadaire ID "+commissioningItoa(id))
		RespondJSON(c, http.StatusOK, gin.H{"status": "failed", "lampadaire_id": id})
	}
}

func updateCommissioningStep(db *sql.DB, id, step int, status string) error {
	_, err := db.Exec(`
		UPDATE lampadaires
		SET commissioning_step=$1, commissioning_status=$2, updated_at=NOW()
		WHERE id=$3`, step, status, id)
	return err
}

func commissioningItoa(n int) string {
	return fmt.Sprintf("%d", n)
}
