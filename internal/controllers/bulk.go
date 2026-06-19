package controllers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/services"
)

func logBulkEvent(c *gin.Context, db *sql.DB, action string, ids []int, meta map[string]interface{}) {
	if meta == nil {
		meta = map[string]interface{}{}
	}
	meta["ids"] = ids
	meta["count"] = len(ids)
	services.LogAction(c.Request.Context(), db, services.AuditEvent{
		Action:    action,
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		Metadata:  meta,
	})
}

var validLampStates = map[string]bool{"online": true, "offline": true, "maintenance": true}

// Doit rester aligné avec la contrainte chk_commissioning_status_v2 (schema.go)
var validCommissioningStatuses = map[string]bool{
	"discovered": true, "located": true, "configured": true,
	"tested": true, "commissioned": true, "failed": true,
}

func intArrayPlaceholders(start, count int) string {
	parts := make([]string, count)
	for i := 0; i < count; i++ {
		parts[i] = fmt.Sprintf("$%d", start+i)
	}
	return strings.Join(parts, ",")
}

func toAnySlice(ints []int) []any {
	out := make([]any, len(ints))
	for i, v := range ints {
		out[i] = v
	}
	return out
}

// buildLampadaireSetClause builds the SET clause for a bulk lampadaire update.
// Returns an error message (empty = OK) so the handler can reply 400.
func buildLampadaireSetClause(updates map[string]string) (setParts []string, args []any, errMsg string) {
	argIdx := 1
	if v, ok := updates["etat"]; ok {
		if !validLampStates[v] {
			return nil, nil, "etat invalide"
		}
		setParts = append(setParts, fmt.Sprintf("etat=$%d", argIdx))
		args = append(args, v)
		argIdx++
	}
	if v, ok := updates["zone"]; ok {
		setParts = append(setParts, fmt.Sprintf("zone=$%d", argIdx))
		args = append(args, v)
		argIdx++
	}
	if v, ok := updates["commissioning_status"]; ok {
		if !validCommissioningStatuses[v] {
			return nil, nil, "commissioning_status invalide"
		}
		setParts = append(setParts, fmt.Sprintf("commissioning_status=$%d", argIdx))
		args = append(args, v)
	}
	if len(setParts) == 0 {
		return nil, nil, "aucun champ valide à mettre à jour"
	}
	return setParts, args, ""
}

// HandleBulkUpdateLampadaires handles PATCH /api/lampadaires/bulk.
// Body: { "ids": [1,2,3], "updates": { "etat"?: "online", "zone"?: "Zone A", "commissioning_status"?: "located" } }
func HandleBulkUpdateLampadaires(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			IDs     []int             `json:"ids"`
			Updates map[string]string `json:"updates"`
		}
		if !BindRequiredJSON(c, &body) {
			return
		}
		if len(body.IDs) == 0 {
			RespondError(c, http.StatusBadRequest, "ids requis")
			return
		}
		if len(body.Updates) == 0 {
			RespondError(c, http.StatusBadRequest, "updates requis")
			return
		}

		setParts, args, errMsg := buildLampadaireSetClause(body.Updates)
		if errMsg != "" {
			RespondError(c, http.StatusBadRequest, errMsg)
			return
		}
		argIdx := len(args) + 1
		setParts = append(setParts, "updated_at=NOW()")

		placeholders := intArrayPlaceholders(argIdx, len(body.IDs))
		args = append(args, toAnySlice(body.IDs)...)

		query := fmt.Sprintf("UPDATE lampadaires SET %s WHERE id IN (%s) AND archived_at IS NULL",
			strings.Join(setParts, ", "), placeholders)

		res, err := db.ExecContext(c.Request.Context(), query, args...)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur mise à jour: "+err.Error())
			return
		}
		n, _ := res.RowsAffected()
		logBulkEvent(c, db, "lampadaires.bulk_update", body.IDs, map[string]interface{}{"updates": body.Updates})
		RespondJSON(c, http.StatusOK, gin.H{"updated": n, "ids": body.IDs})
	}
}

// HandleBulkArchiveLampadaires handles POST /api/lampadaires/bulk/archive.
func HandleBulkArchiveLampadaires(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			IDs []int `json:"ids"`
		}
		if !BindRequiredJSON(c, &body) {
			return
		}
		if len(body.IDs) == 0 {
			RespondError(c, http.StatusBadRequest, "ids requis")
			return
		}
		placeholders := intArrayPlaceholders(1, len(body.IDs))
		query := fmt.Sprintf("UPDATE lampadaires SET archived_at=NOW(), updated_at=NOW() WHERE id IN (%s) AND archived_at IS NULL", placeholders)
		res, err := db.ExecContext(c.Request.Context(), query, toAnySlice(body.IDs)...)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur archivage: "+err.Error())
			return
		}
		n, _ := res.RowsAffected()
		logBulkEvent(c, db, "lampadaires.bulk_archive", body.IDs, nil)
		RespondJSON(c, http.StatusOK, gin.H{"archived": n})
	}
}

// HandleBulkAlertAction handles POST /api/alerts/bulk-action.
// Body: { "ids": [...], "action": "ack"|"resolve"|"close" }
func HandleBulkAlertAction(db *sql.DB) gin.HandlerFunc {
	actions := map[string]struct {
		set      string
		fromStat string
		toStat   string
	}{
		"ack":     {"status='acknowledged', acknowledged_at=NOW()", "open", "acknowledged"},
		"resolve": {"status='resolved', resolved_at=NOW()", "open", "resolved"},
		"close":   {"status='closed', closed_at=NOW()", "any", "closed"},
	}
	return func(c *gin.Context) {
		var body struct {
			IDs    []int  `json:"ids"`
			Action string `json:"action"`
		}
		if !BindRequiredJSON(c, &body) {
			return
		}
		if len(body.IDs) == 0 {
			RespondError(c, http.StatusBadRequest, "ids requis")
			return
		}
		def, ok := actions[body.Action]
		if !ok {
			RespondError(c, http.StatusBadRequest, "action invalide (ack, resolve, close)")
			return
		}
		placeholders := intArrayPlaceholders(1, len(body.IDs))
		where := fmt.Sprintf("id IN (%s)", placeholders)
		if def.fromStat != "any" {
			where += " AND status='" + def.fromStat + "'"
		}
		query := "UPDATE alerts SET " + def.set + " WHERE " + where
		res, err := db.ExecContext(c.Request.Context(), query, toAnySlice(body.IDs)...)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur bulk: "+err.Error())
			return
		}
		n, _ := res.RowsAffected()
		logBulkEvent(c, db, "alerts.bulk_"+body.Action, body.IDs, nil)
		RespondJSON(c, http.StatusOK, gin.H{"updated": n, "action": body.Action, "new_status": def.toStat})
	}
}

// HandleBulkAssignWorkOrders handles POST /api/workorders/bulk-assign.
// Body: { "ids": [...], "user_id": 5 }
func HandleBulkAssignWorkOrders(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			IDs    []int `json:"ids"`
			UserID int   `json:"user_id"`
		}
		if !BindRequiredJSON(c, &body) {
			return
		}
		if len(body.IDs) == 0 || body.UserID <= 0 {
			RespondError(c, http.StatusBadRequest, "ids et user_id requis")
			return
		}
		var userExists bool
		if err := db.QueryRowContext(c.Request.Context(),
			"SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND COALESCE(status,'active') != 'deleted')",
			body.UserID).Scan(&userExists); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur vérification utilisateur")
			return
		}
		if !userExists {
			RespondError(c, http.StatusNotFound, "Utilisateur introuvable")
			return
		}
		placeholders := intArrayPlaceholders(2, len(body.IDs))
		query := fmt.Sprintf(
			"UPDATE work_orders SET assigned_to=$1, status='assigned', updated_at=NOW() WHERE id IN (%s)",
			placeholders)
		args := append([]any{body.UserID}, toAnySlice(body.IDs)...)
		res, err := db.ExecContext(c.Request.Context(), query, args...)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur bulk: "+err.Error())
			return
		}
		n, _ := res.RowsAffected()
		logBulkEvent(c, db, "workorders.bulk_assign", body.IDs, map[string]interface{}{"assigned_to": body.UserID})
		RespondJSON(c, http.StatusOK, gin.H{"assigned": n, "user_id": body.UserID})
	}
}
