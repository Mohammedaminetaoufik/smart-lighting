package controllers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/services"
)

// MaintenanceWindow is the wire representation of a maintenance window.
type MaintenanceWindow struct {
	ID             int       `json:"id"`
	Zone           string    `json:"zone,omitempty"`
	LampadaireIDs  []int     `json:"lampadaire_ids,omitempty"`
	StartsAt       time.Time `json:"starts_at"`
	EndsAt         time.Time `json:"ends_at"`
	Reason         string    `json:"reason,omitempty"`
	CreatedBy      *int      `json:"created_by,omitempty"`
	CreatedByName  string    `json:"created_by_name,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	Active         bool      `json:"active"`
}

// HandleGetMaintenanceWindows handles GET /api/maintenance-windows.
func HandleGetMaintenanceWindows(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT m.id, COALESCE(m.zone, ''), m.lampadaire_ids, m.starts_at, m.ends_at,
				COALESCE(m.reason, ''), m.created_by, COALESCE(u.full_name, ''), m.created_at
			FROM maintenance_windows m
			LEFT JOIN users u ON u.id = m.created_by
			WHERE m.ends_at >= NOW() - INTERVAL '7 days'
			ORDER BY m.starts_at DESC`)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur chargement: "+err.Error())
			return
		}
		defer rows.Close()

		now := time.Now()
		windows := []MaintenanceWindow{}
		for rows.Next() {
			var w MaintenanceWindow
			var idsRaw sql.NullString
			var createdBy sql.NullInt64
			if err := rows.Scan(&w.ID, &w.Zone, &idsRaw, &w.StartsAt, &w.EndsAt,
				&w.Reason, &createdBy, &w.CreatedByName, &w.CreatedAt); err != nil {
				continue
			}
			if createdBy.Valid {
				id := int(createdBy.Int64)
				w.CreatedBy = &id
			}
			if idsRaw.Valid && idsRaw.String != "" {
				_ = json.Unmarshal([]byte(idsRaw.String), &w.LampadaireIDs)
			}
			w.Active = !w.StartsAt.After(now) && !w.EndsAt.Before(now)
			windows = append(windows, w)
		}
		RespondJSON(c, http.StatusOK, windows)
	}
}

// HandleCreateMaintenanceWindow handles POST /api/maintenance-windows.
// Body: { zone?, lampadaire_ids?: [int], starts_at, ends_at, reason? }
func HandleCreateMaintenanceWindow(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Zone          string    `json:"zone"`
			LampadaireIDs []int     `json:"lampadaire_ids"`
			StartsAt      time.Time `json:"starts_at"`
			EndsAt        time.Time `json:"ends_at"`
			Reason        string    `json:"reason"`
		}
		if !BindRequiredJSON(c, &body) {
			return
		}
		body.Zone = strings.TrimSpace(body.Zone)
		body.Reason = strings.TrimSpace(body.Reason)

		if body.StartsAt.IsZero() || body.EndsAt.IsZero() {
			RespondError(c, http.StatusBadRequest, "starts_at et ends_at requis")
			return
		}
		if !body.EndsAt.After(body.StartsAt) {
			RespondError(c, http.StatusBadRequest, "ends_at doit être après starts_at")
			return
		}
		if body.Zone == "" && len(body.LampadaireIDs) == 0 {
			RespondError(c, http.StatusBadRequest, "zone ou lampadaire_ids requis")
			return
		}

		var idsJSON []byte
		if len(body.LampadaireIDs) > 0 {
			idsJSON, _ = json.Marshal(body.LampadaireIDs)
		}

		var id int
		err := db.QueryRowContext(c.Request.Context(), `
			INSERT INTO maintenance_windows (zone, lampadaire_ids, starts_at, ends_at, reason)
			VALUES (NULLIF($1, ''), NULLIF($2, '')::jsonb, $3, $4, NULLIF($5, ''))
			RETURNING id`,
			body.Zone, string(idsJSON), body.StartsAt, body.EndsAt, body.Reason).Scan(&id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur création: "+err.Error())
			return
		}

		services.LogAction(c.Request.Context(), db, services.AuditEvent{
			Action:     "maintenance.create",
			TargetType: "maintenance_window",
			TargetID:   &id,
			IPAddress:  c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
			Metadata: map[string]interface{}{
				"zone":       body.Zone,
				"starts_at":  body.StartsAt,
				"ends_at":    body.EndsAt,
				"lamp_count": len(body.LampadaireIDs),
			},
		})

		RespondJSON(c, http.StatusCreated, gin.H{"id": id, "status": "created"})
	}
}

// HandleDeleteMaintenanceWindow handles DELETE /api/maintenance-windows/:id.
func HandleDeleteMaintenanceWindow(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, errInvalidID)
			return
		}
		res, err := db.ExecContext(c.Request.Context(),
			"DELETE FROM maintenance_windows WHERE id=$1", id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			RespondError(c, http.StatusNotFound, "Fenêtre introuvable")
			return
		}
		services.LogAction(c.Request.Context(), db, services.AuditEvent{
			Action:     "maintenance.delete",
			TargetType: "maintenance_window",
			TargetID:   &id,
			IPAddress:  c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
		})
		RespondJSON(c, http.StatusOK, gin.H{"status": "deleted", "id": id})
	}
}
