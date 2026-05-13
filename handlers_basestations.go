package main

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

func handleGetBasestations(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		list, err := listBasestations(db)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		if list == nil {
			list = []Basestation{}
		}
		respondJSON(c, http.StatusOK, list)
	}
}

func handleCreateBasestation(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var b Basestation
		if err := c.BindJSON(&b); err != nil {
			respondError(c, http.StatusBadRequest, "JSON invalide")
			return
		}
		if b.Reference == "" {
			respondError(c, http.StatusBadRequest, "Le champ reference est obligatoire")
			return
		}
		if err := insertBasestation(db, &b); err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors de la création")
			return
		}
		respondJSON(c, http.StatusCreated, b)
	}
}

func handleGetBasestation(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		b, err := getBasestationByID(db, id)
		if err != nil {
			respondError(c, http.StatusNotFound, "Basestation introuvable")
			return
		}
		respondJSON(c, http.StatusOK, b)
	}
}

func handleSimulateBasestationOffline(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		b, err := getBasestationByID(db, id)
		if err != nil {
			respondError(c, http.StatusNotFound, "Basestation introuvable")
			return
		}
		if err := updateBasestationStatus(db, id, "offline"); err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur mise à jour")
			return
		}
		createAlertIfNotExists(context.Background(), db, 0, "basestation_offline", "major",
			"Basestation "+b.Reference+" hors ligne (simulation)")
		respondJSON(c, http.StatusOK, gin.H{"status": "simulated_offline", "basestation_id": id})
	}
}

func handleGetBasestationControllers(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		list, err := listControllersByBasestation(db, id)
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
