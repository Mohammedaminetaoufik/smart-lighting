package controllers

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/models"
	"map-interactif/internal/repository"
)

// HandleGetCabinets handles GET /api/cabinets.
func HandleGetCabinets(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		list, err := repository.ListCabinets(db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		if list == nil {
			list = []models.Cabinet{}
		}
		RespondJSON(c, http.StatusOK, list)
	}
}

// HandleCreateCabinet handles POST /api/cabinets.
func HandleCreateCabinet(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var cab models.Cabinet
		if err := c.BindJSON(&cab); err != nil {
			RespondError(c, http.StatusBadRequest, "JSON invalide")
			return
		}
		if cab.Reference == "" {
			RespondError(c, http.StatusBadRequest, "Le champ reference est obligatoire")
			return
		}
		if err := repository.InsertCabinet(db, &cab); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors de la création")
			return
		}
		RespondJSON(c, http.StatusCreated, cab)
	}
}

// HandleGetCabinet handles GET /api/cabinets/:id.
func HandleGetCabinet(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		cab, err := repository.GetCabinetByID(db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "Armoire introuvable")
			return
		}
		RespondJSON(c, http.StatusOK, cab)
	}
}

// HandleGetCabinetCircuits handles GET /api/cabinets/:id/circuits.
func HandleGetCabinetCircuits(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		list, err := repository.ListCabinetCircuits(db, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		if list == nil {
			list = []models.CabinetCircuit{}
		}
		RespondJSON(c, http.StatusOK, list)
	}
}

// HandleCreateCabinetCircuit handles POST /api/cabinets/:id/circuits.
func HandleCreateCabinetCircuit(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var cc models.CabinetCircuit
		if err := c.BindJSON(&cc); err != nil {
			RespondError(c, http.StatusBadRequest, "JSON invalide")
			return
		}
		cc.CabinetID = id
		if cc.Name == "" {
			RespondError(c, http.StatusBadRequest, "Le champ name est obligatoire")
			return
		}
		if err := repository.InsertCabinetCircuit(db, &cc); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors de la création")
			return
		}
		RespondJSON(c, http.StatusCreated, cc)
	}
}

// HandleSimulateCabinetDoorOpen handles POST /api/cabinets/:id/simulate-door-open.
func HandleSimulateCabinetDoorOpen(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		cab, err := repository.GetCabinetByID(db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "Armoire introuvable")
			return
		}
		now := time.Now()
		if _, err := db.Exec(`UPDATE cabinets SET door_status='open', last_seen_at=$1, updated_at=$1 WHERE id=$2`, now, id); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur mise à jour")
			return
		}
		repository.CreateAlertIfNotExists(context.Background(), db, 0, "cabinet_door_open", "warning",
			"Porte ouverte sur l'armoire "+cab.Reference)
		RespondJSON(c, http.StatusOK, gin.H{"status": "door_open_simulated", "cabinet_id": id})
	}
}

// HandleSimulatePowerFailure handles POST /api/cabinets/:id/simulate-power-failure.
func HandleSimulatePowerFailure(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		cab, err := repository.GetCabinetByID(db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "Armoire introuvable")
			return
		}
		now := time.Now()
		if _, err := db.Exec(`UPDATE cabinets SET power_status='outage', status='offline', last_seen_at=$1, updated_at=$1 WHERE id=$2`, now, id); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur mise à jour")
			return
		}
		repository.CreateAlertIfNotExists(context.Background(), db, 0, "main_power_failure", "critical",
			"Coupure alimentation sur l'armoire "+cab.Reference)
		RespondJSON(c, http.StatusOK, gin.H{"status": "power_failure_simulated", "cabinet_id": id})
	}
}
