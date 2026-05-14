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

		result, err := services.SyncLCU(c.Request.Context(), db, adapter, id)
		if err != nil {
			RespondError(c, http.StatusServiceUnavailable, "Erreur synchronisation: "+err.Error())
			return
		}

		RespondJSON(c, http.StatusOK, gin.H{
			"message":         result.Message,
			"discovered":      result.DiscoveredCount,
			"created":         result.CreatedCount,
			"updated":         result.UpdatedCount,
			"failed":          result.FailedCount,
			"with_controller": result.ControllerCount,
		})
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
