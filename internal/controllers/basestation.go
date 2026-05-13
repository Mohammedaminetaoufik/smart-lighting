package controllers

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/models"
	"map-interactif/internal/repository"
)

// HandleGetBasestations handles GET /api/basestations.
func HandleGetBasestations(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		list, err := repository.ListBasestations(db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		if list == nil {
			list = []models.Basestation{}
		}
		RespondJSON(c, http.StatusOK, list)
	}
}

// HandleCreateBasestation handles POST /api/basestations.
func HandleCreateBasestation(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var b models.Basestation
		if err := c.BindJSON(&b); err != nil {
			RespondError(c, http.StatusBadRequest, "JSON invalide")
			return
		}
		if b.Reference == "" {
			RespondError(c, http.StatusBadRequest, "Le champ reference est obligatoire")
			return
		}
		if err := repository.InsertBasestation(db, &b); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors de la création")
			return
		}
		RespondJSON(c, http.StatusCreated, b)
	}
}

// HandleGetBasestation handles GET /api/basestations/:id.
func HandleGetBasestation(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		b, err := repository.GetBasestationByID(db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "Basestation introuvable")
			return
		}
		RespondJSON(c, http.StatusOK, b)
	}
}

// HandleSimulateBasestationOffline handles POST /api/basestations/:id/simulate-offline.
func HandleSimulateBasestationOffline(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		b, err := repository.GetBasestationByID(db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "Basestation introuvable")
			return
		}
		if err := repository.UpdateBasestationStatus(db, id, "offline"); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur mise à jour")
			return
		}
		repository.CreateAlertIfNotExists(context.Background(), db, 0, "basestation_offline", "major",
			"Basestation "+b.Reference+" hors ligne (simulation)")
		RespondJSON(c, http.StatusOK, gin.H{"status": "simulated_offline", "basestation_id": id})
	}
}

// HandleGetBasestationControllers handles GET /api/basestations/:id/controllers.
func HandleGetBasestationControllers(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		list, err := repository.ListControllersByBasestation(db, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		if list == nil {
			list = []models.Controller{}
		}
		RespondJSON(c, http.StatusOK, list)
	}
}
