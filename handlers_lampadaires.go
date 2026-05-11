package main

import (
	"database/sql"
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func handleIndex(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		data := PageData{
			Success: c.Query("success") == "1",
		}
		renderIndex(c, db, data)
	}
}

func handleCreateLampadaire(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		form := readForm(c)
		lampadaire, errors := buildLampadaire(form)
		if len(errors) > 0 {
			renderIndex(c, db, PageData{Errors: errors, Form: form})
			return
		}

		if err := insertLampadaire(c.Request.Context(), db, lampadaire); err != nil {
			renderIndex(c, db, PageData{Errors: []string{"Erreur lors de l'enregistrement."}, Form: form})
			return
		}

		c.Redirect(http.StatusSeeOther, "/?success=1")
	}
}

func handleUpdateLampadaire(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		form := readForm(c)
		lampadaire, errors := buildLampadaire(form)
		if len(errors) > 0 {
			renderIndex(c, db, PageData{Errors: errors, Form: form})
			return
		}

		id, err := strconv.Atoi(c.Param("id"))
		if err != nil || id <= 0 {
			renderIndex(c, db, PageData{Errors: []string{"Identifiant invalide."}, Form: form})
			return
		}

		lampadaire.ID = id
		if err := updateLampadaire(c.Request.Context(), db, lampadaire); err != nil {
			renderIndex(c, db, PageData{Errors: []string{"Erreur lors de la mise a jour."}, Form: form})
			return
		}

		c.Redirect(http.StatusSeeOther, "/?success=1")
	}
}

func handleArchiveLampadaire(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "Identifiant invalide.")
			return
		}

		_, err = db.ExecContext(c.Request.Context(), "UPDATE lampadaires SET archived_at = NOW() WHERE id = $1", id)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors de l'archivage.")
			return
		}

		c.Redirect(http.StatusSeeOther, "/?success=1")
	}
}

func handleRestoreLampadaire(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "Identifiant invalide.")
			return
		}

		_, err = db.ExecContext(c.Request.Context(), "UPDATE lampadaires SET archived_at = NULL WHERE id = $1", id)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors de la restauration.")
			return
		}

		c.Redirect(http.StatusSeeOther, "/?success=1")
	}
}

func handleGetLampadaireJSON(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "Identifiant invalide.")
			return
		}

		lamp, err := getLampadaireByID(c.Request.Context(), db, id)
		if err != nil {
			respondError(c, http.StatusNotFound, "Lampadaire introuvable.")
			return
		}

		respondJSON(c, http.StatusOK, lamp)
	}
}

func handleGetDashboardStats(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		stats, err := getDashboardStats(c.Request.Context(), db)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors du chargement des statistiques.")
			return
		}
		respondJSON(c, http.StatusOK, stats)
	}
}
func handleListMissingLocation(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		lamps, err := listLampadairesMissingLocation(c.Request.Context(), db)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors du chargement")
			return
		}
		respondJSON(c, http.StatusOK, lamps)
	}
}

func handleUpdateLampadaireLocationAPI(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "Identifiant invalide.")
			return
		}

		var body struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Status    string  `json:"status"`
		}
		if err := c.BindJSON(&body); err != nil {
			respondError(c, http.StatusBadRequest, "Payload JSON invalide.")
			return
		}

		if body.Status == "" {
			body.Status = "confirmed"
		}

		err = updateLampadaireLocation(c.Request.Context(), db, id, body.Latitude, body.Longitude, body.Status)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur mise e jour.")
			return
		}

		respondJSON(c, http.StatusOK, gin.H{"status": "success"})
	}
}
func renderIndex(c *gin.Context, db *sql.DB, data PageData) {
	search := map[string]string{
		"etat":   c.Query("etat"),
		"zone":   c.Query("zone"),
		"driver": c.Query("driver"),
		"q":      c.Query("q"),
	}

	lampadaires, err := listLampadaires(c.Request.Context(), db, search)
	if err != nil {
		c.String(http.StatusInternalServerError, "Erreur base de donnees")
		return
	}
	if lampadaires == nil {
		lampadaires = []Lampadaire{}
	}

	// Fetch archived ones
	archived, err := listLampadaires(c.Request.Context(), db, map[string]string{"archived": "1"})
	if err != nil {
		archived = []Lampadaire{}
	}

	// Fetch LCUs
	lcus, err := listLCUs(c.Request.Context(), db)
	if err != nil {
		lcus = []LCU{}
	}

	payload, err := json.Marshal(lampadaires)
	if err != nil {
		c.String(http.StatusInternalServerError, "Erreur JSON")
		return
	}

	lcuPayload, _ := json.Marshal(lcus)

	data.Lampadaires = lampadaires
	data.ArchivedLampadaires = archived
	data.LampadairesJSON = template.JS(payload)
	data.LCUs = lcus
	data.LCUsJSON = template.JS(lcuPayload)

	c.HTML(http.StatusOK, "index.tmpl", data)
}

func readForm(c *gin.Context) FormData {
	return FormData{
		Reference:        strings.TrimSpace(c.PostForm("reference")),
		Latitude:         strings.TrimSpace(c.PostForm("latitude")),
		Longitude:        strings.TrimSpace(c.PostForm("longitude")),
		Zone:             strings.TrimSpace(c.PostForm("zone")),
		TypeDriver:       strings.TrimSpace(c.PostForm("type_driver")),
		Protocole:        strings.TrimSpace(c.PostForm("protocole")),
		Puissance:        strings.TrimSpace(c.PostForm("puissance")),
		Etat:             strings.TrimSpace(c.PostForm("etat")),
		Intensite:        strings.TrimSpace(c.PostForm("intensite")),
		DateInstallation: strings.TrimSpace(c.PostForm("date_installation")),
		Address:          strings.TrimSpace(c.PostForm("address")),
		Quartier:         strings.TrimSpace(c.PostForm("quartier")),
		LCUReference:     strings.TrimSpace(c.PostForm("lcu_reference")),
		DriverReference:  strings.TrimSpace(c.PostForm("driver_reference")),
		Notes:            strings.TrimSpace(c.PostForm("notes")),

		LCUID:          strings.TrimSpace(c.PostForm("lcu_id")),
		DeviceUID:      strings.TrimSpace(c.PostForm("device_uid")),
		NodeAddress:    strings.TrimSpace(c.PostForm("node_address")),
		LocationStatus: strings.TrimSpace(c.PostForm("location_status")),
	}
}

func buildLampadaire(form FormData) (Lampadaire, []string) {
	var errors []string
	if form.Reference == "" {
		errors = append(errors, "La reference est obligatoire.")
	}
	if form.Latitude == "" || form.Longitude == "" {
		errors = append(errors, "Selectionnez un emplacement sur la carte.")
	}

	lat, latErr := strconv.ParseFloat(form.Latitude, 64)
	lng, lngErr := strconv.ParseFloat(form.Longitude, 64)
	if latErr != nil || lngErr != nil {
		errors = append(errors, "Latitude et longitude doivent etre des nombres valides.")
	} else {
		if lat < -90 || lat > 90 {
			errors = append(errors, "Latitude doit etre entre -90 et 90.")
		}
		if lng < -180 || lng > 180 {
			errors = append(errors, "Longitude doit etre entre -180 et 180.")
		}
	}

	if len(errors) > 0 {
		return Lampadaire{}, errors
	}

	puissancePtr, puissanceErr := parseOptionalInt(form.Puissance)
	if puissanceErr != nil {
		return Lampadaire{}, []string{"Puissance invalide."}
	}

	intensite := 0
	if form.Intensite != "" {
		value, err := strconv.Atoi(form.Intensite)
		if err != nil || value < 0 || value > 100 {
			return Lampadaire{}, []string{"Intensite doit etre entre 0 et 100."}
		}
		intensite = value
	}

	etat := form.Etat
	if etat == "" {
		etat = "offline"
	}
	validEtat := map[string]bool{"online": true, "offline": true, "maintenance": true}
	if !validEtat[etat] {
		return Lampadaire{}, []string{"Etat doit etre online, offline ou maintenance."}
	}

	di, dateErr := parseOptionalDate(form.DateInstallation)
	if dateErr != nil {
		return Lampadaire{}, []string{"Date installation invalide."}
	}

	status := form.LocationStatus
	if status == "" {
		status = "manual"
	}

	var lcuIDPtr *int
	if form.LCUID != "" {
		lid, err := strconv.Atoi(form.LCUID)
		if err == nil && lid > 0 {
			lcuIDPtr = &lid
		}
	}

	return Lampadaire{
		Reference:        form.Reference,
		Latitude:         lat,
		Longitude:        lng,
		Zone:             form.Zone,
		TypeDriver:       form.TypeDriver,
		Protocole:        form.Protocole,
		Puissance:        puissancePtr,
		Etat:             etat,
		Intensite:        intensite,
		DateInstallation: di,
		Address:          form.Address,
		Quartier:         form.Quartier,
		LCUReference:     form.LCUReference,
		DriverReference:  form.DriverReference,
		Notes:            form.Notes,
		LCUID:            lcuIDPtr,
		DeviceUID:        form.DeviceUID,
		NodeAddress:      form.NodeAddress,
		LocationStatus:   status,
	}, nil
}

func handleGetMissingLocationLampadaires(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		lampadaires, err := listLampadaires(c.Request.Context(), db, map[string]string{})
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur base de donnees")
			return
		}
		missing := []Lampadaire{}
		for _, l := range lampadaires {
			// we assume location is missing if 0,0 but let's base on some logic later
			if l.Latitude == 0 && l.Longitude == 0 {
				missing = append(missing, l)
			}
		}
		respondJSON(c, http.StatusOK, missing)
	}
}

func handleUpdateLampadaireLocation(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var payload struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		}
		if err := c.BindJSON(&payload); err != nil {
			respondError(c, http.StatusBadRequest, "Invalid JSON")
			return
		}
		_, err = db.ExecContext(c.Request.Context(), "UPDATE lampadaires SET latitude=$1, longitude=$2, location_status='confirmed' WHERE id=$3", payload.Latitude, payload.Longitude, id)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		respondJSON(c, http.StatusOK, map[string]string{"status": "success"})
	}
}
