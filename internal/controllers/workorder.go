package controllers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/models"
	"map-interactif/internal/repository"
	"map-interactif/internal/services"
)

// HandleGetWorkOrders handles GET /api/workorders.
func HandleGetWorkOrders(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		list, err := repository.ListWorkOrders(db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		if list == nil {
			list = []models.WorkOrder{}
		}
		RespondJSON(c, http.StatusOK, list)
	}
}

// HandleCreateWorkOrder handles POST /api/workorders.
func HandleCreateWorkOrder(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var wo models.WorkOrder
		if err := c.BindJSON(&wo); err != nil {
			RespondError(c, http.StatusBadRequest, "JSON invalide")
			return
		}
		if wo.Title == "" {
			RespondError(c, http.StatusBadRequest, "Le champ title est obligatoire")
			return
		}
		if err := repository.InsertWorkOrder(db, &wo); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors de la création")
			return
		}
		if len(wo.SourceAlertIDs) > 0 {
			_ = repository.LinkAlertsToWorkOrder(db, wo.ID, wo.SourceAlertIDs)
			for _, aid := range wo.SourceAlertIDs {
				_, _ = db.Exec(`UPDATE alerts SET work_order_id=$1 WHERE id=$2 AND work_order_id IS NULL`, wo.ID, aid)
			}
		}
		ac := services.GetAuditContext(c)
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: ac.UserID, UserName: ac.UserName, UserRole: ac.UserRole,
			Action: "work_order_created", EntityType: "work_order", EntityID: &wo.ID,
			EntityReference: wo.EquipmentReference,
			Description: "Bon de travail créé : " + wo.Title,
			NewValues: map[string]any{"title": wo.Title, "priority": wo.Priority, "status": wo.Status},
			IPAddress: ac.IPAddress, UserAgent: ac.UserAgent,
		})
		RespondJSON(c, http.StatusCreated, wo)
	}
}

// HandleGetWorkOrder handles GET /api/workorders/:id.
func HandleGetWorkOrder(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		wo, err := repository.GetWorkOrderByID(db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "Work order introuvable")
			return
		}
		RespondJSON(c, http.StatusOK, wo)
	}
}

// HandleCreateWorkOrderFromAlerts handles POST /api/workorders/from-alerts.
func HandleCreateWorkOrderFromAlerts(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			AlertIDs []int  `json:"alert_ids"`
			Title    string `json:"title"`
			Priority string `json:"priority"`
			CrewType string `json:"crew_type"`
		}
		if err := c.BindJSON(&body); err != nil || len(body.AlertIDs) == 0 {
			RespondError(c, http.StatusBadRequest, "alert_ids est obligatoire")
			return
		}
		if body.Title == "" {
			body.Title = "Bon de travail groupé"
		}
		wo, err := repository.CreateWorkOrderFromAlerts(db, body.AlertIDs, body.Title, body.Priority, body.CrewType)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors de la création")
			return
		}
		RespondJSON(c, http.StatusCreated, wo)
	}
}

// HandleAssignWorkOrder handles POST /api/workorders/:id/assign.
func HandleAssignWorkOrder(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var body struct {
			UserID int `json:"user_id"`
		}
		if !BindRequiredJSON(c, &body) {
			return
		}
		if body.UserID <= 0 {
			RespondError(c, http.StatusBadRequest, "user_id invalide")
			return
		}
		var userExists bool
		if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND COALESCE(status,'active') != 'deleted')`, body.UserID).Scan(&userExists); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur vérification utilisateur")
			return
		}
		if !userExists {
			RespondError(c, http.StatusNotFound, "Utilisateur introuvable")
			return
		}
		if err := repository.AssignWorkOrder(db, id, body.UserID); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors de l'assignation")
			return
		}
		acA := services.GetAuditContext(c)
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: acA.UserID, UserName: acA.UserName, UserRole: acA.UserRole,
			Action: "work_order_assigned", EntityType: "work_order", EntityID: &id,
			Description: "Bon de travail assigné",
			NewValues: map[string]any{"assigned_user_id": body.UserID},
			IPAddress: acA.IPAddress, UserAgent: acA.UserAgent,
		})
		RespondJSON(c, http.StatusOK, gin.H{"status": "assigned", "work_order_id": id, "user_id": body.UserID})
	}
}

// HandleStartWorkOrder handles POST /api/workorders/:id/start.
func HandleStartWorkOrder(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		if err := repository.UpdateWorkOrderStatus(db, id, "in_progress", ""); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur mise à jour")
			return
		}
		acS := services.GetAuditContext(c)
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: acS.UserID, UserName: acS.UserName, UserRole: acS.UserRole,
			Action: "work_order_started", EntityType: "work_order", EntityID: &id,
			Description: "Bon de travail démarré",
			NewValues: map[string]any{"status": "in_progress"},
			IPAddress: acS.IPAddress, UserAgent: acS.UserAgent,
		})
		RespondJSON(c, http.StatusOK, gin.H{"status": "in_progress", "work_order_id": id})
	}
}

// HandleResolveWorkOrder handles POST /api/workorders/:id/resolve.
func HandleResolveWorkOrder(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var body struct {
			Note string `json:"note"`
		}
		_ = c.BindJSON(&body)
		if err := repository.UpdateWorkOrderStatus(db, id, "resolved", body.Note); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur mise à jour")
			return
		}
		// Resolve all linked open alerts
		_, _ = db.Exec(`
			UPDATE alerts SET status='resolved', resolved_at=NOW()
			WHERE work_order_id=$1 AND status NOT IN ('resolved','closed')`, id)
		acRs := services.GetAuditContext(c)
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: acRs.UserID, UserName: acRs.UserName, UserRole: acRs.UserRole,
			Action: "work_order_resolved", EntityType: "work_order", EntityID: &id,
			Description: "Bon de travail résolu",
			NewValues: map[string]any{"status": "resolved", "note": body.Note},
			IPAddress: acRs.IPAddress, UserAgent: acRs.UserAgent,
		})
		RespondJSON(c, http.StatusOK, gin.H{"status": "resolved", "work_order_id": id})
	}
}

// HandleCloseWorkOrder handles POST /api/workorders/:id/close.
func HandleCloseWorkOrder(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var body struct {
			Note string `json:"note"`
		}
		_ = c.BindJSON(&body)
		if err := repository.UpdateWorkOrderStatus(db, id, "closed", body.Note); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur mise à jour")
			return
		}
		acCl := services.GetAuditContext(c)
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: acCl.UserID, UserName: acCl.UserName, UserRole: acCl.UserRole,
			Action: "work_order_closed", EntityType: "work_order", EntityID: &id,
			Description: "Bon de travail clôturé",
			NewValues: map[string]any{"status": "closed", "note": body.Note},
			IPAddress: acCl.IPAddress, UserAgent: acCl.UserAgent,
		})
		RespondJSON(c, http.StatusOK, gin.H{"status": "closed", "work_order_id": id})
	}
}

// HandleCancelWorkOrder handles POST /api/workorders/:id/cancel.
func HandleCancelWorkOrder(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var body struct {
			Note string `json:"note"`
		}
		_ = c.BindJSON(&body)
		if err := repository.UpdateWorkOrderStatus(db, id, "cancelled", body.Note); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur mise à jour")
			return
		}
		acCan := services.GetAuditContext(c)
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: acCan.UserID, UserName: acCan.UserName, UserRole: acCan.UserRole,
			Action: "work_order_cancelled", EntityType: "work_order", EntityID: &id,
			Description: "Bon de travail annulé",
			NewValues: map[string]any{"status": "cancelled", "note": body.Note},
			IPAddress: acCan.IPAddress, UserAgent: acCan.UserAgent,
		})
		RespondJSON(c, http.StatusOK, gin.H{"status": "cancelled", "work_order_id": id})
	}
}

// HandleGetWorkOrderAlerts handles GET /api/workorders/:id/alerts.
func HandleGetWorkOrderAlerts(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		alerts, err := repository.GetWorkOrderLinkedAlerts(db, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		if alerts == nil {
			alerts = []models.Alert{}
		}
		RespondJSON(c, http.StatusOK, alerts)
	}
}

// HandleGetWorkOrderInterventions handles GET /api/workorders/:id/interventions.
func HandleGetWorkOrderInterventions(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		items, err := repository.GetWorkOrderInterventions(db, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		if items == nil {
			items = []models.Intervention{}
		}
		RespondJSON(c, http.StatusOK, items)
	}
}

// HandleCreateWorkOrderIntervention handles POST /api/workorders/:id/interventions.
func HandleCreateWorkOrderIntervention(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		woID, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		// Verify work order exists
		var exists bool
		if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM work_orders WHERE id=$1)`, woID).Scan(&exists); err != nil || !exists {
			RespondError(c, http.StatusNotFound, "Bon de travail introuvable")
			return
		}

		var iv models.Intervention
		if err := c.BindJSON(&iv); err != nil {
			RespondError(c, http.StatusBadRequest, "JSON invalide")
			return
		}
		if iv.Title == "" {
			iv.Title = "Intervention terrain"
		}
		iv.WorkOrderID = &woID

		// Fetch lampadaire_id from work order
		var lampID sql.NullInt64
		_ = db.QueryRow(`SELECT lampadaire_id FROM work_orders WHERE id=$1`, woID).Scan(&lampID)
		if lampID.Valid {
			v := int(lampID.Int64)
			iv.LampadaireID = &v
		}

		err = db.QueryRow(`
			INSERT INTO interventions (work_order_id, lampadaire_id, assigned_to, technician_name, title, description, action_taken, note, priority, status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'open')
			RETURNING id, created_at, updated_at`,
			woID, iv.LampadaireID, iv.AssignedTo, iv.TechnicianName,
			iv.Title, iv.Description, iv.ActionTaken, iv.Note,
			defaultStr(iv.Priority, "medium"),
		).Scan(&iv.ID, &iv.CreatedAt, &iv.UpdatedAt)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur création intervention: "+err.Error())
			return
		}

		// Mark work order as in_progress if it was open/assigned
		_, _ = db.Exec(`UPDATE work_orders SET status='in_progress', updated_at=NOW() WHERE id=$1 AND status IN ('open','assigned')`, woID)

		RespondJSON(c, http.StatusCreated, iv)
	}
}

// HandleGetOpenWorkOrders handles GET /api/workorders/open.
func HandleGetOpenWorkOrders(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		list, err := repository.GetOpenWorkOrders(db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		if list == nil {
			list = []models.WorkOrder{}
		}
		RespondJSON(c, http.StatusOK, list)
	}
}

// HandleAcceptWorkOrder handles POST /api/workorders/:id/accept.
func HandleAcceptWorkOrder(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var body struct {
			TechnicianID   int    `json:"technician_id"`
			TechnicianName string `json:"technician_name"`
		}
		if !BindRequiredJSON(c, &body) {
			return
		}
		if body.TechnicianName == "" {
			RespondError(c, http.StatusBadRequest, "technician_name est obligatoire")
			return
		}
		if err := repository.AcceptWorkOrder(db, id, body.TechnicianID, body.TechnicianName); err != nil {
			RespondError(c, http.StatusConflict, err.Error())
			return
		}
		acAcc := services.GetAuditContext(c)
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: acAcc.UserID, UserName: acAcc.UserName, UserRole: acAcc.UserRole,
			Action: "work_order_accepted", EntityType: "work_order", EntityID: &id,
			Description: "Bon de travail accepté par " + body.TechnicianName,
			NewValues: map[string]any{"status": "accepted", "technician_name": body.TechnicianName},
			IPAddress: acAcc.IPAddress, UserAgent: acAcc.UserAgent,
		})
		RespondJSON(c, http.StatusOK, gin.H{"status": "accepted", "work_order_id": id})
	}
}

// HandleAddWorkOrderNote handles POST /api/workorders/:id/add-note.
func HandleAddWorkOrderNote(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var body struct {
			Note     string `json:"note"`
			UserName string `json:"user_name"`
		}
		if !BindRequiredJSON(c, &body) {
			return
		}
		if body.Note == "" {
			RespondError(c, http.StatusBadRequest, "note est obligatoire")
			return
		}
		if err := repository.AddWorkOrderNote(db, id, body.Note, body.UserName); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors de l'ajout de la note")
			return
		}
		acN := services.GetAuditContext(c)
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: acN.UserID, UserName: acN.UserName, UserRole: acN.UserRole,
			Action: "work_order_note_added", EntityType: "work_order", EntityID: &id,
			Description: "Note ajoutée au bon de travail",
			NewValues: map[string]any{"note": body.Note},
			IPAddress: acN.IPAddress, UserAgent: acN.UserAgent,
		})
		RespondJSON(c, http.StatusOK, gin.H{"work_order_id": id, "message": "Note ajoutée"})
	}
}

// HandleReopenWorkOrder handles POST /api/workorders/:id/reopen.
func HandleReopenWorkOrder(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var body struct {
			Note string `json:"note"`
		}
		_ = c.BindJSON(&body)
		if err := repository.ReopenWorkOrder(db, id, body.Note); err != nil {
			RespondError(c, http.StatusConflict, err.Error())
			return
		}
		acRo := services.GetAuditContext(c)
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: acRo.UserID, UserName: acRo.UserName, UserRole: acRo.UserRole,
			Action: "work_order_reopened", EntityType: "work_order", EntityID: &id,
			Description: "Bon de travail réouvert",
			NewValues: map[string]any{"status": "open", "note": body.Note},
			IPAddress: acRo.IPAddress, UserAgent: acRo.UserAgent,
		})
		RespondJSON(c, http.StatusOK, gin.H{"status": "open", "work_order_id": id})
	}
}

// HandleGetWorkOrderLogs handles GET /api/workorders/:id/logs.
func HandleGetWorkOrderLogs(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		logs, err := repository.GetWorkOrderLogs(db, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		if logs == nil {
			logs = []models.WorkOrderLog{}
		}
		RespondJSON(c, http.StatusOK, logs)
	}
}

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
