package controllers

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/services"
)

// LampadaireImportRow is a single row in a CSV import.
type LampadaireImportRow struct {
	Reference string   `json:"reference"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
	Zone      string   `json:"zone,omitempty"`
	Puissance *int     `json:"puissance,omitempty"`
	Etat      string   `json:"etat,omitempty"`
	Address   string   `json:"address,omitempty"`
	Quartier  string   `json:"quartier,omitempty"`
}

// HandleImportLampadaires handles POST /api/lampadaires/import.
// Body: { rows: [{ reference, latitude?, longitude?, zone?, ...}, ...] }
// Upserts by reference. Returns counts + per-row errors.
func HandleImportLampadaires(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Rows []LampadaireImportRow `json:"rows"`
		}
		if !BindRequiredJSON(c, &body) {
			return
		}
		if len(body.Rows) == 0 {
			RespondError(c, http.StatusBadRequest, "rows requis")
			return
		}
		if len(body.Rows) > 5000 {
			RespondError(c, http.StatusBadRequest, "Maximum 5000 lignes par import")
			return
		}

		validEtat := map[string]bool{"online": true, "offline": true, "maintenance": true}

		var created, updated int
		errors := []gin.H{}

		for i, r := range body.Rows {
			r.Reference = strings.TrimSpace(r.Reference)
			if r.Reference == "" {
				errors = append(errors, gin.H{"row": i + 1, "error": "reference manquante"})
				continue
			}
			r.Zone = strings.TrimSpace(r.Zone)
			r.Etat = strings.TrimSpace(strings.ToLower(r.Etat))
			if r.Etat == "" {
				r.Etat = "offline"
			}
			if !validEtat[r.Etat] {
				errors = append(errors, gin.H{"row": i + 1, "reference": r.Reference, "error": "etat invalide"})
				continue
			}

			// Validate coordinates if provided
			if r.Latitude != nil && (*r.Latitude < -90 || *r.Latitude > 90) {
				errors = append(errors, gin.H{"row": i + 1, "reference": r.Reference, "error": "latitude hors plage"})
				continue
			}
			if r.Longitude != nil && (*r.Longitude < -180 || *r.Longitude > 180) {
				errors = append(errors, gin.H{"row": i + 1, "reference": r.Reference, "error": "longitude hors plage"})
				continue
			}

			locStatus := "manual"
			if r.Latitude == nil || r.Longitude == nil {
				locStatus = "missing"
			} else {
				locStatus = "confirmed"
			}

			// Upsert by reference
			var existingID int
			err := db.QueryRowContext(c.Request.Context(),
				"SELECT id FROM lampadaires WHERE reference=$1 AND archived_at IS NULL",
				r.Reference).Scan(&existingID)

			if err == sql.ErrNoRows {
				// Insert
				_, err := db.ExecContext(c.Request.Context(), `
					INSERT INTO lampadaires (
						reference, latitude, longitude, zone, puissance, etat,
						address, quartier, intensite, location_status, commissioning_status,
						discovered_by_lcu
					) VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, NULLIF($7, ''), NULLIF($8, ''), 0, $9, 'discovered', false)`,
					r.Reference, r.Latitude, r.Longitude, r.Zone, r.Puissance, r.Etat,
					r.Address, r.Quartier, locStatus)
				if err != nil {
					errors = append(errors, gin.H{"row": i + 1, "reference": r.Reference, "error": err.Error()})
					continue
				}
				created++
			} else if err == nil {
				// Update
				_, err := db.ExecContext(c.Request.Context(), `
					UPDATE lampadaires SET
						latitude = COALESCE($2, latitude),
						longitude = COALESCE($3, longitude),
						zone = COALESCE(NULLIF($4, ''), zone),
						puissance = COALESCE($5, puissance),
						etat = $6,
						address = COALESCE(NULLIF($7, ''), address),
						quartier = COALESCE(NULLIF($8, ''), quartier),
						location_status = CASE
							WHEN $9::text != '' THEN $9::text
							ELSE location_status
						END,
						updated_at = NOW()
					WHERE id = $1`,
					existingID, r.Latitude, r.Longitude, r.Zone, r.Puissance, r.Etat,
					r.Address, r.Quartier, locStatus)
				if err != nil {
					errors = append(errors, gin.H{"row": i + 1, "reference": r.Reference, "error": err.Error()})
					continue
				}
				updated++
			} else {
				errors = append(errors, gin.H{"row": i + 1, "reference": r.Reference, "error": "DB error: " + err.Error()})
				continue
			}
		}

		services.LogAction(c.Request.Context(), db, services.AuditEvent{
			Action:    "lampadaires.import",
			IPAddress: c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
			Metadata: map[string]interface{}{
				"total":   len(body.Rows),
				"created": created,
				"updated": updated,
				"errors":  len(errors),
			},
		})

		RespondJSON(c, http.StatusOK, gin.H{
			"total":   len(body.Rows),
			"created": created,
			"updated": updated,
			"errors":  errors,
		})
	}
}
