package controllers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/models"
	"map-interactif/internal/repository"
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
		}
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
			body.Title = "Work order depuis alertes"
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
		RespondJSON(c, http.StatusOK, gin.H{"status": "closed", "work_order_id": id})
	}
}
