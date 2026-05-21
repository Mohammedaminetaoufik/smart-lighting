package controllers

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/models"
	"map-interactif/internal/repository"
)

type batchTestResult struct {
	ID        int    `json:"id"`
	Reference string `json:"reference"`
	Zone      string `json:"zone,omitempty"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
	Passed    bool   `json:"passed"`
}

type batchSummary struct {
	Total             int               `json:"total"`
	Tested            int               `json:"tested"`
	Passed            int               `json:"passed"`
	Failed            int               `json:"failed"`
	PendingLocation   int               `json:"pending_location"`
	CommissionedReady int               `json:"commissioned_ready"`
	Results           []batchTestResult `json:"results"`
}

// evaluateCommissioningStatus determines what commissioning status a lamp should have
// based on its current technical data, without any external calls.
func evaluateCommissioningStatus(l models.Lampadaire) (status string, step int, reason string) {
	// Critical: no LCU assigned
	if l.LCUID == nil || *l.LCUID == 0 {
		return "failed", stepFailed, "LCU introuvable"
	}
	// Critical: no device UID (not yet synced from LCU)
	if l.DeviceUID == "" {
		return "discovered", stepDiscovered, "Sync LCU requis — Device UID manquant"
	}
	// No confirmed GPS location
	hasGPS := l.Latitude != nil && l.Longitude != nil && (*l.Latitude != 0 || *l.Longitude != 0)
	locationOK := hasGPS && (l.LocationStatus == "confirmed" || l.LocationStatus == "located")
	if !locationOK {
		return "discovered", stepDiscovered, "En attente de localisation GPS"
	}
	// No controller data (not yet reported by the device)
	ctrlType := l.ControllerType
	if ctrlType == "" {
		ctrlType = "Zhaga Book 18"
	}
	if l.ControllerUID == "" {
		return "located", stepLocated, "Données contrôleur manquantes — sync LCU requis"
	}
	// Controller reported as offline
	if l.ControllerStatus != "" && l.ControllerStatus != "ok" {
		return "failed", stepFailed, fmt.Sprintf("Contrôleur offline (statut: %s)", l.ControllerStatus)
	}
	// Signal quality too low
	if l.ControllerSignalQuality != nil && *l.ControllerSignalQuality < 20 {
		return "configured", stepConfigured, fmt.Sprintf("Signal faible (%d%%)", *l.ControllerSignalQuality)
	}
	// Dimming not enabled
	if !l.DimmingEnabled {
		return "configured", stepConfigured, "Dimming non activé"
	}
	// All checks passed
	return "tested", stepTested, "Communication OK · Dimming OK"
}

func updateCommissioningWithNotes(db *sql.DB, id, step int, status, notes string) error {
	_, err := db.Exec(`
		UPDATE lampadaires
		SET commissioning_step=$1, commissioning_status=$2, commissioning_notes=$3,
		    test_comm_status=CASE WHEN $2='tested' THEN 'ok' WHEN $2='failed' THEN 'failed' ELSE test_comm_status END,
		    test_dimming_status=CASE WHEN $2='tested' THEN 'ok' WHEN $2='failed' THEN 'failed' ELSE test_dimming_status END,
		    updated_at=NOW()
		WHERE id=$4`, step, status, notes, id)
	return err
}

// HandleBatchTest handles POST /api/commissioning/batch-test.
// Body: { scope: "all"|"lcu"|"zone"|"selected"|"failed", lcu_id?, zone?, ids? }
func HandleBatchTest(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Scope string `json:"scope"`
			LCUID int    `json:"lcu_id"`
			Zone  string `json:"zone"`
			IDs   []int  `json:"ids"`
		}
		if err := c.BindJSON(&body); err != nil {
			RespondError(c, http.StatusBadRequest, "Invalid JSON")
			return
		}
		if body.Scope == "" {
			body.Scope = "all"
		}

		lamps, err := repository.ListLampadaires(c.Request.Context(), db, map[string]string{})
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}

		// Build set of selected IDs for fast lookup
		selectedSet := map[int]bool{}
		for _, id := range body.IDs {
			selectedSet[id] = true
		}

		var targets []models.Lampadaire
		for _, l := range lamps {
			if l.CommissioningStatus == "commissioned" {
				continue
			}
			switch body.Scope {
			case "lcu":
				if l.LCUID == nil || *l.LCUID != body.LCUID {
					continue
				}
			case "zone":
				if l.Zone != body.Zone {
					continue
				}
			case "selected":
				if !selectedSet[l.ID] {
					continue
				}
			case "failed":
				if l.CommissioningStatus != "failed" {
					continue
				}
			}
			targets = append(targets, l)
		}

		summary := batchSummary{
			Total:   len(targets),
			Results: make([]batchTestResult, 0, len(targets)),
		}

		for _, l := range targets {
			newStatus, newStep, reason := evaluateCommissioningStatus(l)
			_ = updateCommissioningWithNotes(db, l.ID, newStep, newStatus, reason)

			summary.Tested++
			switch newStatus {
			case "tested":
				summary.Passed++
				summary.CommissionedReady++
			case "failed":
				summary.Failed++
			case "discovered":
				summary.PendingLocation++
			}

			summary.Results = append(summary.Results, batchTestResult{
				ID:        l.ID,
				Reference: l.Reference,
				Zone:      l.Zone,
				Status:    newStatus,
				Reason:    reason,
				Passed:    newStatus == "tested",
			})
		}

		RespondJSON(c, http.StatusOK, summary)
	}
}

// HandleValidateSuccessful handles POST /api/commissioning/validate-successful.
// Advances all "tested" lampadaires to "commissioned".
func HandleValidateSuccessful(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		now := time.Now()
		result, err := db.ExecContext(c.Request.Context(), `
			UPDATE lampadaires
			SET commissioning_step=$1, commissioning_status='commissioned',
			    commissioned_at=$2, commissioning_notes='Validé en lot', updated_at=NOW()
			WHERE commissioning_status='tested' AND archived_at IS NULL`,
			stepCommissioned, now)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		n, _ := result.RowsAffected()
		RespondJSON(c, http.StatusOK, gin.H{"commissioned": n})
	}
}

// HandleRetryFailed handles POST /api/commissioning/retry-failed.
// Resets all "failed" lampadaires back to "discovered" so they can be re-evaluated.
func HandleRetryFailed(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := db.ExecContext(c.Request.Context(), `
			UPDATE lampadaires
			SET commissioning_step=$1, commissioning_status='discovered',
			    commissioning_notes='', test_comm_status='pending', test_dimming_status='pending',
			    updated_at=NOW()
			WHERE commissioning_status='failed' AND archived_at IS NULL`,
			stepDiscovered)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		n, _ := result.RowsAffected()
		RespondJSON(c, http.StatusOK, gin.H{"retried": n})
	}
}
