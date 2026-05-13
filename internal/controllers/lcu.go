package controllers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/models"
	"map-interactif/internal/repository"
	"map-interactif/internal/services"
)

// HandleListLCUs serves the LCU page.
func HandleListLCUs(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		renderLCUs(c, db, models.PageData{Success: c.Query("success") == "1"})
	}
}

// HandleCreateLCU handles POST /lcus.
func HandleCreateLCU(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		form := readLCUForm(c)
		lcu, errors := buildLCU(form)
		if len(errors) > 0 {
			renderLCUs(c, db, models.PageData{Errors: errors, LCUForm: form})
			return
		}
		if err := repository.InsertLCU(c.Request.Context(), db, lcu); err != nil {
			renderLCUs(c, db, models.PageData{Errors: []string{"Erreur lors de l'enregistrement de la LCU."}, LCUForm: form})
			return
		}
		c.Redirect(http.StatusSeeOther, "/lcus?success=1")
	}
}

// HandleCreateLCUJSON handles POST /api/lcus.
func HandleCreateLCUJSON(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var lcu models.LCU
		if err := c.BindJSON(&lcu); err != nil {
			RespondError(c, http.StatusBadRequest, "Invalid JSON")
			return
		}
		if lcu.Reference == "" {
			RespondError(c, http.StatusBadRequest, "Reference requise")
			return
		}
		if lcu.Status == "" {
			lcu.Status = "unknown"
		}
		result, err := repository.UpsertLCUByReference(c.Request.Context(), db, lcu)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors de l'enregistrement")
			return
		}
		RespondJSON(c, http.StatusCreated, result)
	}
}

// HandleListLCUsJSON handles GET /api/lcus.
func HandleListLCUsJSON(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		lcus, err := repository.ListLCUs(c.Request.Context(), db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur base de donnees")
			return
		}
		RespondJSON(c, http.StatusOK, lcus)
	}
}

// HandleGetLCUJSON handles GET /api/lcus/:id.
func HandleGetLCUJSON(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		lcu, err := repository.GetLCUByID(c.Request.Context(), db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "LCU introuvable")
			return
		}
		RespondJSON(c, http.StatusOK, lcu)
	}
}

// HandleUpdateLCU handles POST /lcus/:id.
func HandleUpdateLCU(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}

		form := readLCUForm(c)
		lcu, errors := buildLCU(form)
		if len(errors) > 0 {
			renderLCUs(c, db, models.PageData{Errors: errors, LCUForm: form})
			return
		}

		lcu.ID = id
		if err := repository.UpdateLCU(c.Request.Context(), db, lcu); err != nil {
			renderLCUs(c, db, models.PageData{Errors: []string{"Erreur lors de la mise e jour de la LCU."}, LCUForm: form})
			return
		}

		c.Redirect(http.StatusSeeOther, "/lcus?success=1")
	}
}

// HandleTestLCU handles POST /api/lcus/:id/test.
func HandleTestLCU(db *sql.DB, adapter services.LCUAdapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}

		lcu, err := repository.GetLCUByID(c.Request.Context(), db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "LCU introuvable")
			return
		}

		err = adapter.Health(c.Request.Context(), lcu)
		status := "online"
		if err != nil {
			status = "offline"
		}

		repository.UpdateLCUStatus(c.Request.Context(), db, id, status)

		if err != nil {
			RespondError(c, http.StatusServiceUnavailable, "echec du test : "+err.Error())
			return
		}

		RespondJSON(c, http.StatusOK, gin.H{"status": "online", "message": "Connexion etablie avec succes"})
	}
}

// HandleSyncLCU handles POST /api/lcus/:id/sync.
func HandleSyncLCU(db *sql.DB, adapter services.LCUAdapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}

		lcu, err := repository.GetLCUByID(c.Request.Context(), db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "LCU introuvable")
			return
		}

		devices, err := adapter.DiscoverDevices(c.Request.Context(), lcu)
		if err != nil {
			log := models.LCUSyncLog{
				LCUID:   id,
				Status:  "failed",
				Message: "echec decouverte : " + err.Error(),
			}
			repository.InsertLCUSyncLog(c.Request.Context(), db, log)
			RespondError(c, http.StatusServiceUnavailable, log.Message)
			return
		}

		discovered := len(devices)
		created := 0
		updated := 0
		failed := 0

		tx, err := db.BeginTx(c.Request.Context(), nil)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur transaction")
			return
		}
		defer tx.Rollback()

		for _, d := range devices {
			var existingID int
			var existingCommStatus string
			err := tx.QueryRowContext(c.Request.Context(), "SELECT id, commissioning_status FROM lampadaires WHERE lcu_id = $1 AND device_uid = $2", id, d.DeviceUID).Scan(&existingID, &existingCommStatus)

			locStatus := "confirmed"
			lat := d.Latitude
			lng := d.Longitude

			if lat == nil || lng == nil {
				locStatus = "estimated"
				lat = lcu.Latitude
				lng = lcu.Longitude
			}

			commStatus := "discovered"
			if existingCommStatus != "" {
				commStatus = existingCommStatus
			}
			if commStatus == "discovered" && (lat != nil && lng != nil) {
				commStatus = "located"
			}

			lamp := models.Lampadaire{
				Reference:           d.Reference,
				Latitude:            lat,
				Longitude:           lng,
				Zone:                d.Zone,
				TypeDriver:          d.TypeDriver,
				Protocole:           d.Protocole,
				Puissance:           d.Puissance,
				Etat:                d.Etat,
				Intensite:           d.Intensite,
				LCUID:               &id,
				DeviceUID:           d.DeviceUID,
				NodeAddress:         d.NodeAddress,
				DiscoveredByLCU:     true,
				LocationStatus:      locStatus,
				CommissioningStatus: commStatus,
			}

			if err == sql.ErrNoRows {
				if err := repository.InsertLampadaireTx(c.Request.Context(), tx, lamp); err != nil {
					failed++
				} else {
					created++
				}
			} else if err == nil {
				lamp.ID = existingID
				if err := repository.UpdateLampadaireTx(c.Request.Context(), tx, lamp); err != nil {
					failed++
				} else {
					updated++
				}
			} else {
				failed++
			}
		}

		syncLog := models.LCUSyncLog{
			LCUID:           id,
			Status:          "success",
			Message:         fmt.Sprintf("Synchronisation terminee : %d decouverts, %d cres, %d mis e jour, %d erreurs", discovered, created, updated, failed),
			DiscoveredCount: discovered,
			CreatedCount:    created,
			UpdatedCount:    updated,
			FailedCount:     failed,
		}
		repository.InsertLCUSyncLogTx(c.Request.Context(), tx, syncLog)

		tx.ExecContext(c.Request.Context(), "UPDATE lcus SET last_sync_at = NOW() WHERE id = $1", id)

		if err := tx.Commit(); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur commit")
			return
		}

		RespondJSON(c, http.StatusOK, syncLog)
	}
}

// HandleGetLCULampadaires handles GET /api/lcus/:id/lampadaires.
func HandleGetLCULampadaires(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		lampadaires, err := repository.ListLampadairesByLCU(c.Request.Context(), db, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		RespondJSON(c, http.StatusOK, lampadaires)
	}
}

func renderLCUs(c *gin.Context, db *sql.DB, data models.PageData) {
	lcus, err := repository.ListLCUs(c.Request.Context(), db)
	if err != nil {
		c.String(http.StatusInternalServerError, "Erreur base de donnees")
		return
	}

	payload, _ := json.Marshal(lcus)
	data.LCUs = lcus
	data.LCUsJSON = template.JS(payload)

	c.HTML(http.StatusOK, "index.tmpl", data)
}

func readLCUForm(c *gin.Context) models.LCUFormData {
	return models.LCUFormData{
		Reference: strings.TrimSpace(c.PostForm("reference")),
		Name:      strings.TrimSpace(c.PostForm("name")),
		IPAddress: strings.TrimSpace(c.PostForm("ip_address")),
		Port:      strings.TrimSpace(c.PostForm("port")),
		Protocol:  strings.TrimSpace(c.PostForm("protocol")),
		AuthToken: strings.TrimSpace(c.PostForm("auth_token")),
		Zone:      strings.TrimSpace(c.PostForm("zone")),
		Address:   strings.TrimSpace(c.PostForm("address")),
		Latitude:  strings.TrimSpace(c.PostForm("latitude")),
		Longitude: strings.TrimSpace(c.PostForm("longitude")),
	}
}

func buildLCU(form models.LCUFormData) (models.LCU, []string) {
	var errors []string
	if form.Reference == "" {
		errors = append(errors, "La reference est obligatoire.")
	}
	if form.IPAddress == "" {
		errors = append(errors, "L'adresse IP est obligatoire.")
	}

	port := 8080
	if form.Port != "" {
		p, err := strconv.Atoi(form.Port)
		if err != nil || p <= 0 {
			errors = append(errors, "Port invalide.")
		} else {
			port = p
		}
	}

	var lat, lng *float64
	if form.Latitude != "" {
		l, err := strconv.ParseFloat(form.Latitude, 64)
		if err == nil {
			lat = &l
		}
	}
	if form.Longitude != "" {
		l, err := strconv.ParseFloat(form.Longitude, 64)
		if err == nil {
			lng = &l
		}
	}

	if len(errors) > 0 {
		return models.LCU{}, errors
	}

	protocol := form.Protocol
	if protocol == "" {
		protocol = "HTTP"
	}

	return models.LCU{
		Reference: form.Reference,
		Name:      form.Name,
		IPAddress: form.IPAddress,
		Port:      port,
		Protocol:  protocol,
		AuthToken: form.AuthToken,
		Zone:      form.Zone,
		Address:   form.Address,
		Latitude:  lat,
		Longitude: lng,
		Status:    "offline",
	}, nil
}
