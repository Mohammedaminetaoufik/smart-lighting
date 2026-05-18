package controllers

import (
	"database/sql"
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/models"
	"map-interactif/internal/repository"
	"map-interactif/internal/services"
	"map-interactif/internal/utils"
)

// HandleIndex serves the main page.
func HandleIndex(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		data := models.PageData{
			Success: c.Query("success") == "1",
		}
		RenderIndex(c, db, data)
	}
}

// HandleCreateLampadaire handles POST /lampadaires.
func HandleCreateLampadaire(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		form := ReadForm(c)
		lampadaire, errors := BuildLampadaire(form)
		if len(errors) > 0 {
			RenderIndex(c, db, models.PageData{Errors: errors, Form: form})
			return
		}

		if err := repository.InsertLampadaire(c.Request.Context(), db, lampadaire); err != nil {
			RenderIndex(c, db, models.PageData{Errors: []string{"Erreur lors de l'enregistrement."}, Form: form})
			return
		}

		c.Redirect(http.StatusSeeOther, "/?success=1")
	}
}

// HandleUpdateLampadaire handles POST /lampadaires/:id.
func HandleUpdateLampadaire(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		form := ReadForm(c)
		lampadaire, errors := BuildLampadaire(form)
		if len(errors) > 0 {
			RenderIndex(c, db, models.PageData{Errors: errors, Form: form})
			return
		}

		id, err := strconv.Atoi(c.Param("id"))
		if err != nil || id <= 0 {
			RenderIndex(c, db, models.PageData{Errors: []string{"Identifiant invalide."}, Form: form})
			return
		}

		lampadaire.ID = id
		if err := repository.UpdateLampadaire(c.Request.Context(), db, lampadaire); err != nil {
			RenderIndex(c, db, models.PageData{Errors: []string{"Erreur lors de la mise a jour."}, Form: form})
			return
		}

		c.Redirect(http.StatusSeeOther, "/?success=1")
	}
}

// HandleArchiveLampadaire handles POST /lampadaires/:id/archive.
func HandleArchiveLampadaire(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "Identifiant invalide.")
			return
		}

		_, err = db.ExecContext(c.Request.Context(), "UPDATE lampadaires SET archived_at = NOW() WHERE id = $1", id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors de l'archivage.")
			return
		}

		c.Redirect(http.StatusSeeOther, "/?success=1")
	}
}

// HandleRestoreLampadaire handles POST /lampadaires/:id/restore.
func HandleRestoreLampadaire(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "Identifiant invalide.")
			return
		}

		_, err = db.ExecContext(c.Request.Context(), "UPDATE lampadaires SET archived_at = NULL WHERE id = $1", id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors de la restauration.")
			return
		}

		c.Redirect(http.StatusSeeOther, "/?success=1")
	}
}

// HandleListLampadairesJSON handles GET /api/lampadaires — lists all non-archived lampadaires.
func HandleListLampadairesJSON(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		filters := map[string]string{}
		if etat := c.Query("etat"); etat != "" {
			filters["etat"] = etat
		}
		if zone := c.Query("zone"); zone != "" {
			filters["zone"] = zone
		}
		if q := c.Query("q"); q != "" {
			filters["q"] = q
		}
		lamps, err := repository.ListLampadaires(c.Request.Context(), db, filters)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		if lamps == nil {
			lamps = []models.Lampadaire{}
		}
		RespondJSON(c, http.StatusOK, lamps)
	}
}

// HandleGetLampadaireJSON handles GET /api/lampadaires/:id.
func HandleGetLampadaireJSON(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "Identifiant invalide.")
			return
		}

		lamp, err := repository.GetLampadaireByID(c.Request.Context(), db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "Lampadaire introuvable.")
			return
		}

		RespondJSON(c, http.StatusOK, lamp)
	}
}

// HandleListMissingLocation handles GET /api/lampadaires/missing-location.
func HandleListMissingLocation(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		lamps, err := repository.ListLampadairesMissingLocation(c.Request.Context(), db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors du chargement")
			return
		}
		RespondJSON(c, http.StatusOK, lamps)
	}
}

// HandleGetMissingLocationLampadaires handles lampadaires with missing/estimated locations.
func HandleGetMissingLocationLampadaires(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		lampadaires, err := repository.ListLampadaires(c.Request.Context(), db, map[string]string{})
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur base de donnees")
			return
		}
		missing := []models.Lampadaire{}
		for _, l := range lampadaires {
			noCoords := l.Latitude == nil || l.Longitude == nil || (*l.Latitude == 0 && *l.Longitude == 0)
			estimated := l.LocationStatus == "estimated"
			if noCoords || estimated {
				missing = append(missing, l)
			}
		}
		RespondJSON(c, http.StatusOK, missing)
	}
}

// HandleUpdateLampadaireLocation handles POST /api/lampadaires/:id/location.
func HandleUpdateLampadaireLocation(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var payload struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		}
		if err := c.BindJSON(&payload); err != nil {
			RespondError(c, http.StatusBadRequest, "Invalid JSON")
			return
		}
		if payload.Latitude < -90 || payload.Latitude > 90 {
			RespondError(c, http.StatusBadRequest, "Latitude invalide (doit être entre -90 et 90)")
			return
		}
		if payload.Longitude < -180 || payload.Longitude > 180 {
			RespondError(c, http.StatusBadRequest, "Longitude invalide (doit être entre -180 et 180)")
			return
		}
		_, err = db.ExecContext(c.Request.Context(), `
			UPDATE lampadaires
			SET latitude=$1, longitude=$2, location_status='confirmed',
			commissioning_status = CASE WHEN commissioning_status = 'discovered' THEN 'located' ELSE commissioning_status END
			WHERE id=$3`, payload.Latitude, payload.Longitude, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		RespondJSON(c, http.StatusOK, map[string]string{"status": "success"})
	}
}

// HandleUpdateCommissioningStatus handles POST /api/lampadaires/:id/commissioning.
func HandleUpdateCommissioningStatus(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var payload struct {
			Status string `json:"commissioning_status"`
		}
		if err := c.BindJSON(&payload); err != nil {
			RespondError(c, http.StatusBadRequest, "Invalid JSON")
			return
		}
		if payload.Status == "" {
			RespondError(c, http.StatusBadRequest, "Le statut ne peut pas être vide")
			return
		}
		_, err = db.ExecContext(c.Request.Context(), "UPDATE lampadaires SET commissioning_status=$1 WHERE id=$2", payload.Status, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Database error: "+err.Error())
			return
		}
		RespondJSON(c, http.StatusOK, map[string]string{"status": "success"})
	}
}

// HandleAssignLCU handles POST /api/lampadaires/:id/assign-lcu.
// Body: { "lcu_id": 5 } or { "lcu_id": null } to unassign.
func HandleAssignLCU(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var body struct {
			LCUID *int `json:"lcu_id"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			RespondError(c, http.StatusBadRequest, "JSON invalide")
			return
		}
		if body.LCUID != nil {
			var exists bool
			db.QueryRowContext(c.Request.Context(), `SELECT EXISTS(SELECT 1 FROM lcus WHERE id=$1)`, *body.LCUID).Scan(&exists)
			if !exists {
				RespondError(c, http.StatusBadRequest, "LCU introuvable")
				return
			}
			if _, err := db.ExecContext(c.Request.Context(),
				`UPDATE lampadaires SET lcu_id=$1, updated_at=NOW() WHERE id=$2`, *body.LCUID, id); err != nil {
				RespondError(c, http.StatusInternalServerError, err.Error())
				return
			}
		} else {
			if _, err := db.ExecContext(c.Request.Context(),
				`UPDATE lampadaires SET lcu_id=NULL, updated_at=NOW() WHERE id=$1`, id); err != nil {
				RespondError(c, http.StatusInternalServerError, err.Error())
				return
			}
		}
		RespondJSON(c, http.StatusOK, gin.H{"lampadaire_id": id, "lcu_id": body.LCUID})
	}
}

// HandleDiagnoseLampadaire handles GET /api/lampadaires/:id/diagnostic.
func HandleDiagnoseLampadaire(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		lamp, err := repository.GetLampadaireByID(c.Request.Context(), db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "Lampadaire introuvable")
			return
		}
		result := services.DiagnoseLampadaireIssue(lamp)
		RespondJSON(c, http.StatusOK, result)
	}
}

// RenderIndex renders the main index template.
func RenderIndex(c *gin.Context, db *sql.DB, data models.PageData) {
	search := map[string]string{
		"etat":   c.Query("etat"),
		"zone":   c.Query("zone"),
		"driver": c.Query("driver"),
		"q":      c.Query("q"),
	}

	lampadaires, err := repository.ListLampadaires(c.Request.Context(), db, search)
	if err != nil {
		c.String(http.StatusInternalServerError, "Erreur base de donnees")
		return
	}
	if lampadaires == nil {
		lampadaires = []models.Lampadaire{}
	}

	archived, err := repository.ListLampadaires(c.Request.Context(), db, map[string]string{"archived": "1"})
	if err != nil {
		archived = []models.Lampadaire{}
	}

	lcus, err := repository.ListLCUs(c.Request.Context(), db)
	if err != nil {
		lcus = []models.LCU{}
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

// ReadForm reads lampadaire form data from the request.
func ReadForm(c *gin.Context) models.FormData {
	return models.FormData{
		Reference:           strings.TrimSpace(c.PostForm("reference")),
		Latitude:            strings.TrimSpace(c.PostForm("latitude")),
		Longitude:           strings.TrimSpace(c.PostForm("longitude")),
		Zone:                strings.TrimSpace(c.PostForm("zone")),
		TypeDriver:          strings.TrimSpace(c.PostForm("type_driver")),
		Protocole:           strings.TrimSpace(c.PostForm("protocole")),
		Puissance:           strings.TrimSpace(c.PostForm("puissance")),
		Etat:                strings.TrimSpace(c.PostForm("etat")),
		Intensite:           strings.TrimSpace(c.PostForm("intensite")),
		DateInstallation:    strings.TrimSpace(c.PostForm("date_installation")),
		Address:             strings.TrimSpace(c.PostForm("address")),
		Quartier:            strings.TrimSpace(c.PostForm("quartier")),
		LCUReference:        strings.TrimSpace(c.PostForm("lcu_reference")),
		DriverReference:     strings.TrimSpace(c.PostForm("driver_reference")),
		Notes:               strings.TrimSpace(c.PostForm("notes")),
		LCUID:               strings.TrimSpace(c.PostForm("lcu_id")),
		DeviceUID:           strings.TrimSpace(c.PostForm("device_uid")),
		NodeAddress:         strings.TrimSpace(c.PostForm("node_address")),
		LocationStatus:      strings.TrimSpace(c.PostForm("location_status")),
		CommissioningStatus: strings.TrimSpace(c.PostForm("commissioning_status")),
	}
}

// BuildLampadaire builds a Lampadaire from form data.
func BuildLampadaire(form models.FormData) (models.Lampadaire, []string) {
	var errors []string
	if form.Reference == "" {
		errors = append(errors, "La reference est obligatoire.")
	}
	if form.Latitude == "" || form.Longitude == "" {
		errors = append(errors, "Selectionnez un emplacement sur la carte.")
	}

	var latPtr, lngPtr *float64
	if form.Latitude != "" {
		lat, err := strconv.ParseFloat(form.Latitude, 64)
		if err != nil || lat < -90 || lat > 90 {
			errors = append(errors, "Latitude invalide.")
		} else {
			latPtr = &lat
		}
	}
	if form.Longitude != "" {
		lng, err := strconv.ParseFloat(form.Longitude, 64)
		if err != nil || lng < -180 || lng > 180 {
			errors = append(errors, "Longitude invalide.")
		} else {
			lngPtr = &lng
		}
	}

	if len(errors) > 0 {
		return models.Lampadaire{}, errors
	}

	puissancePtr, puissanceErr := utils.ParseOptionalInt(form.Puissance)
	if puissanceErr != nil {
		return models.Lampadaire{}, []string{"Puissance invalide."}
	}

	intensite := 0
	if form.Intensite != "" {
		value, err := strconv.Atoi(form.Intensite)
		if err != nil || value < 0 || value > 100 {
			return models.Lampadaire{}, []string{"Intensite doit etre entre 0 et 100."}
		}
		intensite = value
	}

	etat := form.Etat
	if etat == "" {
		etat = "offline"
	}
	validEtat := map[string]bool{"online": true, "offline": true, "maintenance": true}
	if !validEtat[etat] {
		return models.Lampadaire{}, []string{"Etat doit etre online, offline ou maintenance."}
	}

	di, dateErr := utils.ParseOptionalDate(form.DateInstallation)
	if dateErr != nil {
		return models.Lampadaire{}, []string{"Date installation invalide."}
	}

	status := form.LocationStatus
	if status == "" {
		status = "manual"
	}

	commStatus := form.CommissioningStatus
	if commStatus == "" {
		commStatus = "discovered"
	}

	var lcuIDPtr *int
	if form.LCUID != "" {
		lid, err := strconv.Atoi(form.LCUID)
		if err == nil && lid > 0 {
			lcuIDPtr = &lid
		}
	}

	return models.Lampadaire{
		Reference:           form.Reference,
		Latitude:            latPtr,
		Longitude:           lngPtr,
		Zone:                form.Zone,
		TypeDriver:          form.TypeDriver,
		Protocole:           form.Protocole,
		Puissance:           puissancePtr,
		Etat:                etat,
		Intensite:           intensite,
		DateInstallation:    di,
		Address:             form.Address,
		Quartier:            form.Quartier,
		LCUReference:        form.LCUReference,
		DriverReference:     form.DriverReference,
		Notes:               form.Notes,
		LCUID:               lcuIDPtr,
		DeviceUID:           form.DeviceUID,
		NodeAddress:         form.NodeAddress,
		LocationStatus:      status,
		CommissioningStatus: commStatus,
	}, nil
}
