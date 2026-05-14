package controllers

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/models"
	"map-interactif/internal/repository"
	"map-interactif/internal/services"
)

// HandleGetLightingProfiles handles GET /api/lighting-profiles.
func HandleGetLightingProfiles(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		profiles, err := repository.ListLightingProfiles(db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		RespondJSON(c, http.StatusOK, profiles)
	}
}

// HandleCreateLightingProfile handles POST /api/lighting-profiles.
func HandleCreateLightingProfile(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var p models.LightingProfile
		if err := c.BindJSON(&p); err != nil {
			RespondError(c, http.StatusBadRequest, "Invalid JSON")
			return
		}
		if err := repository.InsertLightingProfile(db, &p); err != nil {
			if strings.HasPrefix(err.Error(), "DUPLICATE:") {
				RespondError(c, http.StatusConflict, "Ce profil existe déjà pour cette cible.")
				return
			}
			RespondError(c, http.StatusInternalServerError, "Erreur lors de l'enregistrement")
			return
		}
		RespondJSON(c, http.StatusCreated, p)
	}
}

// HandleEnableLightingProfile handles POST /api/lighting-profiles/:id/enable.
func HandleEnableLightingProfile(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		if _, err := db.Exec("UPDATE lighting_profiles SET enabled = true WHERE id = $1", id); err != nil {
			RespondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		RespondJSON(c, http.StatusOK, gin.H{"status": "success"})
	}
}

// HandleDisableLightingProfile handles POST /api/lighting-profiles/:id/disable.
func HandleDisableLightingProfile(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		if _, err := db.Exec("UPDATE lighting_profiles SET enabled = false WHERE id = $1", id); err != nil {
			RespondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		RespondJSON(c, http.StatusOK, gin.H{"status": "success"})
	}
}

// HandleApplyLightingProfile handles POST /api/lighting-profiles/:id/apply.
func HandleApplyLightingProfile(db *sql.DB, adapter services.LCUAdapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		p, err := repository.GetLightingProfileByID(db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "Profil introuvable")
			return
		}

		var query string
		var args []interface{}
		switch p.TargetType {
		case "zone", "group":
			query = "SELECT id FROM lampadaires WHERE zone = $1 AND archived_at IS NULL"
			args = append(args, p.TargetValue)
		case "lcu":
			lcuID, err := strconv.Atoi(p.TargetValue)
			if err != nil || lcuID <= 0 {
				err2 := db.QueryRow("SELECT id FROM lcus WHERE reference = $1", p.TargetValue).Scan(&lcuID)
				if err2 != nil || lcuID <= 0 {
					RespondError(c, http.StatusBadRequest, "target_value invalide pour le type lcu")
					return
				}
			}
			query = "SELECT id FROM lampadaires WHERE lcu_id = $1 AND archived_at IS NULL"
			args = append(args, lcuID)
		default:
			RespondError(c, http.StatusBadRequest, "Type de cible inconnu")
			return
		}

		rows, err := db.Query(query, args...)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()

		var lampIDs []int
		for rows.Next() {
			var lid int
			if err := rows.Scan(&lid); err == nil {
				lampIDs = append(lampIDs, lid)
			}
		}

		intensity := 100
		if len(p.Schedules) > 0 {
			intensity = p.Schedules[0].Intensity
		}

		for _, lid := range lampIDs {
			if _, err := db.Exec("INSERT INTO dimming_commands (lampadaire_id, source, new_intensity, reason) VALUES ($1, $2, $3, $4)",
				lid, "profile_eclairage", intensity, "Application manuelle du profil: "+p.Name); err != nil {
				log.Printf("HandleApplyLightingProfile: insert dimming_command for lamp %d failed: %v", lid, err)
				continue
			}
			if _, err := db.Exec("UPDATE lampadaires SET intensite = $1, last_command_at = NOW() WHERE id = $2", intensity, lid); err != nil {
				log.Printf("HandleApplyLightingProfile: update lampadaire %d intensity failed: %v", lid, err)
			}
		}

		RespondJSON(c, http.StatusOK, gin.H{"status": "applied", "count": len(lampIDs)})
	}
}

// HandleGetLightingProfileDetails handles GET /api/lighting-profiles/:id/details.
func HandleGetLightingProfileDetails(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		p, err := repository.GetLightingProfileByID(db, id)
		if err != nil {
			RespondError(c, http.StatusNotFound, "Profil introuvable")
			return
		}

		var query string
		var args []interface{}
		switch p.TargetType {
		case "zone", "group":
			query = `SELECT id, reference, etat, intensite, zone, address, device_uid, lcu_id
			         FROM lampadaires WHERE zone = $1 AND archived_at IS NULL ORDER BY reference`
			args = append(args, p.TargetValue)
		case "lcu":
			lcuID, err := strconv.Atoi(p.TargetValue)
			if err != nil || lcuID <= 0 {
				db.QueryRow("SELECT id FROM lcus WHERE reference = $1", p.TargetValue).Scan(&lcuID)
			}
			query = `SELECT id, reference, etat, intensite, zone, address, device_uid, lcu_id
			         FROM lampadaires WHERE lcu_id = $1 AND archived_at IS NULL ORDER BY reference`
			args = append(args, lcuID)
		default:
			RespondError(c, http.StatusBadRequest, "Type de cible inconnu")
			return
		}

		rows, err := db.Query(query, args...)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()

		type LampSummary struct {
			ID        int    `json:"id"`
			Reference string `json:"reference"`
			Etat      string `json:"etat"`
			Intensite int    `json:"intensite"`
			Zone      string `json:"zone"`
			Address   string `json:"address"`
			DeviceUID string `json:"device_uid"`
			LCUID     *int   `json:"lcu_id"`
			Problem   string `json:"problem,omitempty"`
		}

		var lamps []LampSummary
		for rows.Next() {
			var l LampSummary
			var addr, uid sql.NullString
			if err := rows.Scan(&l.ID, &l.Reference, &l.Etat, &l.Intensite, &l.Zone, &addr, &uid, &l.LCUID); err != nil {
				continue
			}
			l.Address = addr.String
			l.DeviceUID = uid.String
			switch l.Etat {
			case "offline":
				l.Problem = "hors ligne"
			case "maintenance":
				l.Problem = "en maintenance"
			}
			if l.DeviceUID == "" {
				if l.Problem != "" {
					l.Problem += ", UID manquant"
				} else {
					l.Problem = "UID manquant (commande impossible)"
				}
			}
			if l.LCUID == nil {
				if l.Problem != "" {
					l.Problem += ", non associé à une LCU"
				} else {
					l.Problem = "non associé à une LCU"
				}
			}
			lamps = append(lamps, l)
		}

		total := len(lamps)
		problematic := 0
		for _, l := range lamps {
			if l.Problem != "" {
				problematic++
			}
		}

		RespondJSON(c, http.StatusOK, gin.H{
			"profile":     p,
			"lamps":       lamps,
			"total":       total,
			"problematic": problematic,
		})
	}
}

// HandleGetLightingGroups handles GET /api/lighting-groups.
func HandleGetLightingGroups(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.Query("SELECT id, name, zone, description, created_at, updated_at FROM lighting_groups ORDER BY name")
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()
		var groups []models.LightingGroup
		for rows.Next() {
			var g models.LightingGroup
			if err := rows.Scan(&g.ID, &g.Name, &g.Zone, &g.Description, &g.CreatedAt, &g.UpdatedAt); err != nil {
				continue
			}
			groups = append(groups, g)
		}
		RespondJSON(c, http.StatusOK, groups)
	}
}

// HandleCreateLightingGroup handles POST /api/lighting-groups.
func HandleCreateLightingGroup(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var g models.LightingGroup
		if err := c.BindJSON(&g); err != nil {
			RespondError(c, http.StatusBadRequest, "Invalid JSON")
			return
		}
		err := db.QueryRow("INSERT INTO lighting_groups (name, zone, description) VALUES ($1, $2, $3) RETURNING id, created_at, updated_at",
			g.Name, g.Zone, g.Description).Scan(&g.ID, &g.CreatedAt, &g.UpdatedAt)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		RespondJSON(c, http.StatusCreated, g)
	}
}
