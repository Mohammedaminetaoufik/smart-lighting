package main

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

func handleGetControllers(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		list, err := listControllers(db)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		if list == nil {
			list = []Controller{}
		}
		respondJSON(c, http.StatusOK, list)
	}
}

func handleCreateController(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var ctrl Controller
		if err := c.BindJSON(&ctrl); err != nil {
			respondError(c, http.StatusBadRequest, "JSON invalide")
			return
		}
		if ctrl.ControllerUID == "" {
			respondError(c, http.StatusBadRequest, "Le champ controller_uid est obligatoire")
			return
		}
		if err := insertController(db, &ctrl); err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors de la création")
			return
		}
		respondJSON(c, http.StatusCreated, ctrl)
	}
}

func handleGetController(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		ctrl, err := getControllerByID(db, id)
		if err != nil {
			respondError(c, http.StatusNotFound, "Contrôleur introuvable")
			return
		}
		respondJSON(c, http.StatusOK, ctrl)
	}
}

func handleAssociateController(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var body struct {
			LampadaireID int `json:"lampadaire_id"`
		}
		if err := c.BindJSON(&body); err != nil || body.LampadaireID <= 0 {
			respondError(c, http.StatusBadRequest, "lampadaire_id invalide")
			return
		}
		if err := associateControllerToLampadaire(db, id, body.LampadaireID); err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors de l'association")
			return
		}
		respondJSON(c, http.StatusOK, gin.H{"status": "associated", "controller_id": id, "lampadaire_id": body.LampadaireID})
	}
}
