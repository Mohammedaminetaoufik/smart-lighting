package main

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func handleGetLightingProfiles(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		profiles, err := listLightingProfiles(db)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur base de données")
			return
		}
		respondJSON(c, http.StatusOK, profiles)
	}
}

func handleCreateLightingProfile(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var p LightingProfile
		if err := c.BindJSON(&p); err != nil {
			respondError(c, http.StatusBadRequest, "Invalid JSON")
			return
		}
		if err := insertLightingProfile(db, &p); err != nil {
			respondError(c, http.StatusInternalServerError, "Erreur lors de l'enregistrement")
			return
		}
		respondJSON(c, http.StatusCreated, p)
	}
}

func handleEnableLightingProfile(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		if _, err := db.Exec("UPDATE lighting_profiles SET enabled = true WHERE id = $1", id); err != nil {
			respondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		respondJSON(c, http.StatusOK, gin.H{"status": "success"})
	}
}

func handleDisableLightingProfile(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		if _, err := db.Exec("UPDATE lighting_profiles SET enabled = false WHERE id = $1", id); err != nil {
			respondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		respondJSON(c, http.StatusOK, gin.H{"status": "success"})
	}
}

func handleApplyLightingProfile(db *sql.DB, adapter LCUAdapter) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseIDParam(c, "id")
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		p, err := getLightingProfileByID(db, id)
		if err != nil {
			respondError(c, http.StatusNotFound, "Profil introuvable")
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
				respondError(c, http.StatusBadRequest, "target_value invalide pour le type lcu")
				return
			}
			query = "SELECT id FROM lampadaires WHERE lcu_id = $1 AND archived_at IS NULL"
			args = append(args, lcuID)
		default:
			respondError(c, http.StatusBadRequest, "Type de cible inconnu")
			return
		}

		rows, err := db.Query(query, args...)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Database error")
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
				log.Printf("handleApplyLightingProfile: insert dimming_command for lamp %d failed: %v", lid, err)
				continue
			}
			if _, err := db.Exec("UPDATE lampadaires SET intensite = $1, last_command_at = NOW() WHERE id = $2", intensity, lid); err != nil {
				log.Printf("handleApplyLightingProfile: update lampadaire %d intensity failed: %v", lid, err)
			}
		}

		respondJSON(c, http.StatusOK, gin.H{"status": "applied", "count": len(lampIDs)})
	}
}

func handleGetLightingGroups(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.Query("SELECT id, name, zone, description, created_at, updated_at FROM lighting_groups ORDER BY name")
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()
		var groups []LightingGroup
		for rows.Next() {
			var g LightingGroup
			if err := rows.Scan(&g.ID, &g.Name, &g.Zone, &g.Description, &g.CreatedAt, &g.UpdatedAt); err != nil {
				continue
			}
			groups = append(groups, g)
		}
		respondJSON(c, http.StatusOK, groups)
	}
}

func handleCreateLightingGroup(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var g LightingGroup
		if err := c.BindJSON(&g); err != nil {
			respondError(c, http.StatusBadRequest, "Invalid JSON")
			return
		}
		err := db.QueryRow("INSERT INTO lighting_groups (name, zone, description) VALUES ($1, $2, $3) RETURNING id, created_at, updated_at",
			g.Name, g.Zone, g.Description).Scan(&g.ID, &g.CreatedAt, &g.UpdatedAt)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		respondJSON(c, http.StatusCreated, g)
	}
}

// DB Helpers for profiles
func listLightingProfiles(db *sql.DB) ([]LightingProfile, error) {
	rows, err := db.Query("SELECT id, name, description, target_type, target_value, enabled, created_at, updated_at FROM lighting_profiles")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var profiles []LightingProfile
	for rows.Next() {
		var p LightingProfile
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.TargetType, &p.TargetValue, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			continue
		}

		sRows, _ := db.Query("SELECT id, profile_id, start_time, end_time, intensity, days_of_week, created_at FROM lighting_profile_schedules WHERE profile_id = $1", p.ID)
		if sRows != nil {
			for sRows.Next() {
				var s LightingProfileSchedule
				if err := sRows.Scan(&s.ID, &s.ProfileID, &s.StartTime, &s.EndTime, &s.Intensity, &s.DaysOfWeek, &s.CreatedAt); err == nil {
					p.Schedules = append(p.Schedules, s)
				}
			}
			sRows.Close()
		}

		profiles = append(profiles, p)
	}
	return profiles, nil
}

func getLightingProfileByID(db *sql.DB, id int) (*LightingProfile, error) {
	var p LightingProfile
	err := db.QueryRow("SELECT id, name, description, target_type, target_value, enabled, created_at, updated_at FROM lighting_profiles WHERE id = $1", id).
		Scan(&p.ID, &p.Name, &p.Description, &p.TargetType, &p.TargetValue, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	sRows, _ := db.Query("SELECT id, profile_id, start_time, end_time, intensity, days_of_week, created_at FROM lighting_profile_schedules WHERE profile_id = $1", p.ID)
	if sRows != nil {
		for sRows.Next() {
			var s LightingProfileSchedule
			if err := sRows.Scan(&s.ID, &s.ProfileID, &s.StartTime, &s.EndTime, &s.Intensity, &s.DaysOfWeek, &s.CreatedAt); err == nil {
				p.Schedules = append(p.Schedules, s)
			}
		}
		sRows.Close()
	}
	return &p, nil
}

func insertLightingProfile(db *sql.DB, p *LightingProfile) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	err = tx.QueryRow("INSERT INTO lighting_profiles (name, description, target_type, target_value, enabled) VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at, updated_at",
		p.Name, p.Description, p.TargetType, p.TargetValue, p.Enabled).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		tx.Rollback()
		return err
	}
	for _, s := range p.Schedules {
		_, err = tx.Exec("INSERT INTO lighting_profile_schedules (profile_id, start_time, end_time, intensity, days_of_week) VALUES ($1, $2, $3, $4, $5)",
			p.ID, s.StartTime, s.EndTime, s.Intensity, s.DaysOfWeek)
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
