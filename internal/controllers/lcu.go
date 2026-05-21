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
		if lcu.IPAddress == "" {
			lcu.IPAddress = "0.0.0.0"
		}
		if lcu.Port == 0 {
			lcu.Port = 8080
		}
		if lcu.Protocol == "" {
			lcu.Protocol = "HTTP"
		}
		result, err := repository.UpsertLCUByReference(c.Request.Context(), db, lcu)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors de l'enregistrement")
			return
		}
		ac := services.GetAuditContext(c)
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: ac.UserID, UserName: ac.UserName, UserRole: ac.UserRole,
			Action: "lcu_created", EntityType: "lcu", EntityID: &result.ID,
			EntityReference: result.Reference,
			Description: "LCU créée : " + result.Reference,
			NewValues:   map[string]any{"reference": result.Reference, "ip": result.IPAddress, "zone": result.Zone},
			IPAddress: ac.IPAddress, UserAgent: ac.UserAgent,
		})
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

// HandleUpdateLCUJSON handles PUT /api/lcus/:id.
func HandleUpdateLCUJSON(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		existing, err := repository.GetLCUByID(c.Request.Context(), db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "LCU introuvable")
			return
		}
		var patch models.LCU
		if err := c.BindJSON(&patch); err != nil {
			RespondError(c, http.StatusBadRequest, "Invalid JSON")
			return
		}
		patch.ID = id
		patch.Status = existing.Status
		if patch.Port == 0 {
			patch.Port = existing.Port
		}
		if err := repository.UpdateLCU(c.Request.Context(), db, patch); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors de la mise à jour")
			return
		}
		updated, _ := repository.GetLCUByID(c.Request.Context(), db, id)
		ac := services.GetAuditContext(c)
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: ac.UserID, UserName: ac.UserName, UserRole: ac.UserRole,
			Action: "lcu_updated", EntityType: "lcu", EntityID: &id,
			EntityReference: existing.Reference,
			Description: "LCU modifiée : " + existing.Reference,
			OldValues: map[string]any{"ip": existing.IPAddress, "zone": existing.Zone, "protocol": existing.Protocol},
			NewValues: map[string]any{"ip": patch.IPAddress, "zone": patch.Zone, "protocol": patch.Protocol},
			IPAddress: ac.IPAddress, UserAgent: ac.UserAgent,
		})
		RespondJSON(c, http.StatusOK, updated)
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
		ac := services.GetAuditContext(c)
		auditStatus := "success"
		if err != nil {
			auditStatus = "error"
		}
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: ac.UserID, UserName: ac.UserName, UserRole: ac.UserRole,
			Action: "lcu_tested", EntityType: "lcu", EntityID: &id,
			EntityReference: lcu.Reference,
			Description: "Test connexion LCU : " + lcu.Reference + " → " + status,
			NewValues: map[string]any{"status": status},
			Status: auditStatus,
			IPAddress: ac.IPAddress, UserAgent: ac.UserAgent,
		})

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
		ac2 := services.GetAuditContext(c)
		syncStatus := "success"
		syncDesc := "Synchronisation LCU réussie"
		if err != nil {
			syncStatus = "error"
			syncDesc = "Échec synchronisation LCU : " + err.Error()
		}
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: ac2.UserID, UserName: ac2.UserName, UserRole: ac2.UserRole,
			Action: "lcu_synced", EntityType: "lcu", EntityID: &id,
			Description: syncDesc,
			Status: syncStatus,
			IPAddress: ac2.IPAddress, UserAgent: ac2.UserAgent,
		})
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

// HandleBulkDimLCU handles POST /api/lcus/:id/bulk-dim.
// Dims all lampadaires attached to the LCU in one DB operation and writes
// exactly one audit event — regardless of how many lamps are affected.
func HandleBulkDimLCU(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		var body struct {
			Intensity int    `json:"intensity"`
			Reason    string `json:"reason"`
		}
		if !BindRequiredJSON(c, &body) {
			return
		}
		if body.Intensity < 0 || body.Intensity > 100 {
			RespondError(c, http.StatusBadRequest, "L'intensité doit être entre 0 et 100")
			return
		}

		lcu, err := repository.GetLCUByID(c.Request.Context(), db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "LCU introuvable")
			return
		}

		reason := body.Reason
		if reason == "" {
			reason = "Gradation manuelle LCU"
		}

		// Fetch all active lamps for this LCU
		rows, err := db.QueryContext(c.Request.Context(),
			`SELECT id FROM lampadaires WHERE lcu_id=$1 AND archived_at IS NULL`, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lecture lampadaires")
			return
		}
		defer rows.Close()
		var lampIDs []int
		for rows.Next() {
			var lid int
			if rows.Scan(&lid) == nil {
				lampIDs = append(lampIDs, lid)
			}
		}

		if len(lampIDs) == 0 {
			RespondJSON(c, http.StatusOK, gin.H{"updated": 0, "message": "Aucun lampadaire dans cette LCU"})
			return
		}

		// Batch update — single statement using unnest
		_, err = db.ExecContext(c.Request.Context(), `
			UPDATE lampadaires SET intensite=$1, last_command_at=NOW(), updated_at=NOW()
			WHERE lcu_id=$2 AND archived_at IS NULL`, body.Intensity, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur mise à jour intensité: "+err.Error())
			return
		}

		// Insert ONE dimming_command row as a summary record
		db.ExecContext(c.Request.Context(), `
			INSERT INTO dimming_commands (lampadaire_id, source, new_intensity, reason, status, applied_at)
			VALUES ($1, 'bulk_lcu', $2, $3, 'applied', NOW())`,
			lampIDs[0], body.Intensity, reason)

		// ONE audit event for the whole operation
		ac := services.GetAuditContext(c)
		services.LogAudit(c.Request.Context(), db, services.AuditLogInput{
			UserID: ac.UserID, UserName: ac.UserName, UserRole: ac.UserRole,
			Action: "dimming_bulk_lcu", EntityType: "lcu", EntityID: &id,
			EntityReference: lcu.Reference,
			Description: strings.Join([]string{
				"Gradation en masse LCU", lcu.Reference,
				":", strings.TrimSpace(reason),
			}, " "),
			NewValues: map[string]any{
				"intensity":   body.Intensity,
				"lamp_count":  len(lampIDs),
				"lcu_reference": lcu.Reference,
			},
			IPAddress: ac.IPAddress, UserAgent: ac.UserAgent,
		})

		RespondJSON(c, http.StatusOK, gin.H{
			"updated":   len(lampIDs),
			"intensity": body.Intensity,
			"lcu_id":    id,
		})
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
