package controllers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/models"
	"map-interactif/internal/repository"
)

// HandleGetControllers handles GET /api/controllers.
func HandleGetControllers(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		list, err := repository.ListControllers(db)
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

// HandleCreateController handles POST /api/controllers.
func HandleCreateController(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var ctrl models.Controller
		if err := c.BindJSON(&ctrl); err != nil {
			RespondError(c, http.StatusBadRequest, "JSON invalide")
			return
		}
		if ctrl.ControllerUID == "" {
			RespondError(c, http.StatusBadRequest, "Le champ controller_uid est obligatoire")
			return
		}
		if err := repository.InsertController(db, &ctrl); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors de la création")
			return
		}
		RespondJSON(c, http.StatusCreated, ctrl)
	}
}

// HandleGetController handles GET /api/controllers/:id.
func HandleGetController(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		ctrl, err := repository.GetControllerByID(db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "Contrôleur introuvable")
			return
		}
		RespondJSON(c, http.StatusOK, ctrl)
	}
}

// HandleAssociateController handles POST /api/controllers/:id/associate.
func HandleAssociateController(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var body struct {
			LampadaireID int `json:"lampadaire_id"`
		}
		if !BindRequiredJSON(c, &body) {
			return
		}
		if body.LampadaireID <= 0 {
			RespondError(c, http.StatusBadRequest, "lampadaire_id invalide")
			return
		}
		if err := repository.AssociateControllerToLampadaire(db, id, body.LampadaireID); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors de l'association")
			return
		}
		RespondJSON(c, http.StatusOK, gin.H{"status": "associated", "controller_id": id, "lampadaire_id": body.LampadaireID})
	}
}
