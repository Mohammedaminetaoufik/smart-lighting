package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func handleListLCUs(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		renderLCUs(c, db, PageData{Success: c.Query("success") == "1"})
	}
}

func handleCreateLCU(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		form := readLCUForm(c)
		lcu, errors := buildLCU(form)
		if len(errors) > 0 {
			renderLCUs(c, db, PageData{Errors: errors, LCUForm: form})
			return
		}

		if err := insertLCU(c.Request.Context(), db, lcu); err != nil {
			renderLCUs(c, db, PageData{Errors: []string{"Erreur lors de l'enregistrement de la LCU."}, LCUForm: form})
			return
		}

		c.Redirect(http.StatusSeeOther, "/lcus?success=1")
	}
}

func handleListLCUsJSON(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		lcus, err := listLCUs(c.Request.Context(), db)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur base de donnees")
			return
		}
		respondJSON(c, http.StatusOK, lcus)
	}
}

func handleGetLCUJSON(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		lcu, err := getLCUByID(c.Request.Context(), db, id)
		if err != nil {
			respondError(c, http.StatusNotFound, "LCU introuvable")
			return
		}
		respondJSON(c, http.StatusOK, lcu)
	}
}

func handleUpdateLCU(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}

		form := readLCUForm(c)
		lcu, errors := buildLCU(form)
		if len(errors) > 0 {
			renderLCUs(c, db, PageData{Errors: errors, LCUForm: form})
			return
		}

		lcu.ID = id
		if err := updateLCU(c.Request.Context(), db, lcu); err != nil {
			renderLCUs(c, db, PageData{Errors: []string{"Erreur lors de la mise e jour de la LCU."}, LCUForm: form})
			return
		}

		c.Redirect(http.StatusSeeOther, "/lcus?success=1")
	}
}

func handleTestLCU(db *sql.DB, adapter LCUAdapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}

		lcu, err := getLCUByID(c.Request.Context(), db, id)
		if err != nil {
			respondError(c, http.StatusNotFound, "LCU introuvable")
			return
		}

		err = adapter.Health(c.Request.Context(), lcu)
		status := "online"
		if err != nil {
			status = "offline"
		}

		updateLCUStatus(c.Request.Context(), db, id, status)

		if err != nil {
			respondError(c, http.StatusServiceUnavailable, "echec du test : "+err.Error())
			return
		}

		respondJSON(c, http.StatusOK, gin.H{"status": "online", "message": "Connexion etablie avec succes"})
	}
}

func handleSyncLCU(db *sql.DB, adapter LCUAdapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}

		lcu, err := getLCUByID(c.Request.Context(), db, id)
		if err != nil {
			respondError(c, http.StatusNotFound, "LCU introuvable")
			return
		}

		// Sync logic
		devices, err := adapter.DiscoverDevices(c.Request.Context(), lcu)
		if err != nil {
			log := LCUSyncLog{
				LCUID:   id,
				Status:  "failed",
				Message: "echec decouverte : " + err.Error(),
			}
			insertLCUSyncLog(c.Request.Context(), db, log)
			respondError(c, http.StatusServiceUnavailable, log.Message)
			return
		}

		discovered := len(devices)
		created := 0
		updated := 0
		failed := 0

		for _, d := range devices {
			// Find existing lampadaire by device_uid and lcu_id
			var existingID int
			err := db.QueryRowContext(c.Request.Context(), "SELECT id FROM lampadaires WHERE lcu_id = $1 AND device_uid = $2", id, d.DeviceUID).Scan(&existingID)

			locStatus := "confirmed"
			if d.Latitude == nil || d.Longitude == nil {
				locStatus = "missing"
			}

			lamp := Lampadaire{
				Reference:       d.Reference,
				Latitude:        0,
				Longitude:       0,
				Zone:            d.Zone,
				TypeDriver:      d.TypeDriver,
				Protocole:       d.Protocole,
				Puissance:       d.Puissance,
				Etat:            d.Etat,
				Intensite:       d.Intensite,
				LCUID:           &id,
				DeviceUID:       d.DeviceUID,
				NodeAddress:     d.NodeAddress,
				DiscoveredByLCU: true,
				LocationStatus:  locStatus,
			}
			if d.Latitude != nil {
				lamp.Latitude = *d.Latitude
			}
			if d.Longitude != nil {
				lamp.Longitude = *d.Longitude
			}

			if err == sql.ErrNoRows {
				// Insert
				if err := insertLampadaire(c.Request.Context(), db, lamp); err != nil {
					failed++
				} else {
					created++
				}
			} else if err == nil {
				// Update
				lamp.ID = existingID
				if err := updateLampadaire(c.Request.Context(), db, lamp); err != nil {
					failed++
				} else {
					updated++
				}
			} else {
				failed++
			}
		}

		log := LCUSyncLog{
			LCUID:           id,
			Status:          "success",
			Message:         fmt.Sprintf("Synchronisation terminee : %d decouverts, %d cres, %d mis e jour, %d erreurs", discovered, created, updated, failed),
			DiscoveredCount: discovered,
			CreatedCount:    created,
			UpdatedCount:    updated,
			FailedCount:     failed,
		}
		insertLCUSyncLog(c.Request.Context(), db, log)

		// Update LCU last sync
		db.ExecContext(c.Request.Context(), "UPDATE lcus SET last_sync_at = NOW() WHERE id = $1", id)

		respondJSON(c, http.StatusOK, log)
	}
}

func renderLCUs(c *gin.Context, db *sql.DB, data PageData) {
	lcus, err := listLCUs(c.Request.Context(), db)
	if err != nil {
		c.String(http.StatusInternalServerError, "Erreur base de donnees")
		return
	}

	payload, _ := json.Marshal(lcus)
	data.LCUs = lcus
	data.LCUsJSON = template.JS(payload)

	// We'll need to update index.tmpl to handle LCU page
	c.HTML(http.StatusOK, "index.tmpl", data)
}

func readLCUForm(c *gin.Context) LCUFormData {
	return LCUFormData{
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

func buildLCU(form LCUFormData) (LCU, []string) {
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
		return LCU{}, errors
	}

	protocol := form.Protocol
	if protocol == "" {
		protocol = "HTTP"
	}

	return LCU{
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

func handleGetLCULampadaires(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		lampadaires, err := listLampadairesByLCU(c.Request.Context(), db, id)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		respondJSON(c, http.StatusOK, lampadaires)
	}
}
