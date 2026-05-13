package main

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func handleGetCabinets(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		list, err := listCabinets(db)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		if list == nil {
			list = []Cabinet{}
		}
		respondJSON(c, http.StatusOK, list)
	}
}

func handleCreateCabinet(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var cab Cabinet
		if err := c.BindJSON(&cab); err != nil {
			respondError(c, http.StatusBadRequest, "JSON invalide")
			return
		}
		if cab.Reference == "" {
			respondError(c, http.StatusBadRequest, "Le champ reference est obligatoire")
			return
		}
		if err := insertCabinet(db, &cab); err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors de la création")
			return
		}
		respondJSON(c, http.StatusCreated, cab)
	}
}

func handleGetCabinet(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		cab, err := getCabinetByID(db, id)
		if err != nil {
			respondError(c, http.StatusNotFound, "Armoire introuvable")
			return
		}
		respondJSON(c, http.StatusOK, cab)
	}
}

func handleGetCabinetCircuits(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		list, err := listCabinetCircuits(db, id)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		if list == nil {
			list = []CabinetCircuit{}
		}
		respondJSON(c, http.StatusOK, list)
	}
}

func handleCreateCabinetCircuit(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var cc CabinetCircuit
		if err := c.BindJSON(&cc); err != nil {
			respondError(c, http.StatusBadRequest, "JSON invalide")
			return
		}
		cc.CabinetID = id
		if cc.Name == "" {
			respondError(c, http.StatusBadRequest, "Le champ name est obligatoire")
			return
		}
		if err := insertCabinetCircuit(db, &cc); err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors de la création")
			return
		}
		respondJSON(c, http.StatusCreated, cc)
	}
}

func handleSimulateCabinetDoorOpen(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		cab, err := getCabinetByID(db, id)
		if err != nil {
			respondError(c, http.StatusNotFound, "Armoire introuvable")
			return
		}
		now := time.Now()
		if _, err := db.Exec(`UPDATE cabinets SET door_status='open', last_seen_at=$1, updated_at=$1 WHERE id=$2`, now, id); err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur mise à jour")
			return
		}
		createAlertIfNotExists(context.Background(), db, 0, "cabinet_door_open", "warning",
			"Porte ouverte sur l'armoire "+cab.Reference)
		respondJSON(c, http.StatusOK, gin.H{"status": "door_open_simulated", "cabinet_id": id})
	}
}

func handleSimulatePowerFailure(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		cab, err := getCabinetByID(db, id)
		if err != nil {
			respondError(c, http.StatusNotFound, "Armoire introuvable")
			return
		}
		now := time.Now()
		if _, err := db.Exec(`UPDATE cabinets SET power_status='outage', status='offline', last_seen_at=$1, updated_at=$1 WHERE id=$2`, now, id); err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur mise à jour")
			return
		}
		createAlertIfNotExists(context.Background(), db, 0, "main_power_failure", "critical",
			"Coupure alimentation sur l'armoire "+cab.Reference)
		respondJSON(c, http.StatusOK, gin.H{"status": "power_failure_simulated", "cabinet_id": id})
	}
}
