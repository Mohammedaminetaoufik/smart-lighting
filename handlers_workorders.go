package main

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

func handleGetWorkOrders(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		list, err := listWorkOrders(db)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		if list == nil {
			list = []WorkOrder{}
		}
		respondJSON(c, http.StatusOK, list)
	}
}

func handleCreateWorkOrder(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var wo WorkOrder
		if err := c.BindJSON(&wo); err != nil {
			respondError(c, http.StatusBadRequest, "JSON invalide")
			return
		}
		if wo.Title == "" {
			respondError(c, http.StatusBadRequest, "Le champ title est obligatoire")
			return
		}
		if err := insertWorkOrder(db, &wo); err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors de la création")
			return
		}
		if len(wo.SourceAlertIDs) > 0 {
			_ = linkAlertsToWorkOrder(db, wo.ID, wo.SourceAlertIDs)
		}
		respondJSON(c, http.StatusCreated, wo)
	}
}

func handleGetWorkOrder(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		wo, err := getWorkOrderByID(db, id)
		if err != nil {
			respondError(c, http.StatusNotFound, "Work order introuvable")
			return
		}
		respondJSON(c, http.StatusOK, wo)
	}
}

func handleCreateWorkOrderFromAlerts(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			AlertIDs  []int  `json:"alert_ids"`
			Title     string `json:"title"`
			Priority  string `json:"priority"`
			CrewType  string `json:"crew_type"`
		}
		if err := c.BindJSON(&body); err != nil || len(body.AlertIDs) == 0 {
			respondError(c, http.StatusBadRequest, "alert_ids est obligatoire")
			return
		}
		if body.Title == "" {
			body.Title = "Work order depuis alertes"
		}
		wo, err := createWorkOrderFromAlerts(db, body.AlertIDs, body.Title, body.Priority, body.CrewType)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors de la création")
			return
		}
		respondJSON(c, http.StatusCreated, wo)
	}
}

func handleAssignWorkOrder(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var body struct {
			UserID int `json:"user_id"`
		}
		if err := c.BindJSON(&body); err != nil || body.UserID <= 0 {
			respondError(c, http.StatusBadRequest, "user_id invalide")
			return
		}
		if err := assignWorkOrder(db, id, body.UserID); err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors de l'assignation")
			return
		}
		respondJSON(c, http.StatusOK, gin.H{"status": "assigned", "work_order_id": id, "user_id": body.UserID})
	}
}

func handleStartWorkOrder(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		if err := updateWorkOrderStatus(db, id, "in_progress", ""); err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur mise à jour")
			return
		}
		respondJSON(c, http.StatusOK, gin.H{"status": "in_progress", "work_order_id": id})
	}
}

func handleResolveWorkOrder(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var body struct {
			Note string `json:"note"`
		}
		_ = c.BindJSON(&body)
		if err := updateWorkOrderStatus(db, id, "resolved", body.Note); err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur mise à jour")
			return
		}
		respondJSON(c, http.StatusOK, gin.H{"status": "resolved", "work_order_id": id})
	}
}

func handleCloseWorkOrder(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var body struct {
			Note string `json:"note"`
		}
		_ = c.BindJSON(&body)
		if err := updateWorkOrderStatus(db, id, "closed", body.Note); err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur mise à jour")
			return
		}
		respondJSON(c, http.StatusOK, gin.H{"status": "closed", "work_order_id": id})
	}
}
