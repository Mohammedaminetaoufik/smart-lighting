package repository

import (
	"database/sql"
	"fmt"

	"map-interactif/internal/models"
)

// ListLightingProfiles returns all lighting profiles with their schedules.
func ListLightingProfiles(db *sql.DB) ([]models.LightingProfile, error) {
	rows, err := db.Query("SELECT id, name, description, target_type, target_value, enabled, created_at, updated_at FROM lighting_profiles")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var profiles []models.LightingProfile
	for rows.Next() {
		var p models.LightingProfile
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.TargetType, &p.TargetValue, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			continue
		}

		sRows, _ := db.Query("SELECT id, profile_id, start_time, end_time, intensity, days_of_week, created_at FROM lighting_profile_schedules WHERE profile_id = $1", p.ID)
		if sRows != nil {
			for sRows.Next() {
				var s models.LightingProfileSchedule
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

// GetLightingProfileByID fetches a single lighting profile by ID.
func GetLightingProfileByID(db *sql.DB, id int) (*models.LightingProfile, error) {
	var p models.LightingProfile
	err := db.QueryRow("SELECT id, name, description, target_type, target_value, enabled, created_at, updated_at FROM lighting_profiles WHERE id = $1", id).
		Scan(&p.ID, &p.Name, &p.Description, &p.TargetType, &p.TargetValue, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	sRows, _ := db.Query("SELECT id, profile_id, start_time, end_time, intensity, days_of_week, created_at FROM lighting_profile_schedules WHERE profile_id = $1", p.ID)
	if sRows != nil {
		for sRows.Next() {
			var s models.LightingProfileSchedule
			if err := sRows.Scan(&s.ID, &s.ProfileID, &s.StartTime, &s.EndTime, &s.Intensity, &s.DaysOfWeek, &s.CreatedAt); err == nil {
				p.Schedules = append(p.Schedules, s)
			}
		}
		sRows.Close()
	}
	return &p, nil
}

// InsertLightingProfile inserts a lighting profile with its schedules.
func InsertLightingProfile(db *sql.DB, p *models.LightingProfile) error {
	var existingID int
	err := db.QueryRow(`SELECT id FROM lighting_profiles WHERE name = $1 AND target_type = $2 AND target_value = $3`,
		p.Name, p.TargetType, p.TargetValue).Scan(&existingID)
	if err == nil {
		return fmt.Errorf("DUPLICATE:%d", existingID)
	}

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

// UpdateLightingProfile replaces a profile's metadata and schedules.
func UpdateLightingProfile(db *sql.DB, p *models.LightingProfile) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("UPDATE lighting_profiles SET name=$1, description=$2, target_type=$3, target_value=$4, enabled=$5, updated_at=NOW() WHERE id=$6",
		p.Name, p.Description, p.TargetType, p.TargetValue, p.Enabled, p.ID); err != nil {
		return err
	}
	tx.Exec("DELETE FROM lighting_profile_schedules WHERE profile_id=$1", p.ID)
	for _, s := range p.Schedules {
		if _, err = tx.Exec("INSERT INTO lighting_profile_schedules (profile_id, start_time, end_time, intensity, days_of_week) VALUES ($1,$2,$3,$4,$5)",
			p.ID, s.StartTime, s.EndTime, s.Intensity, s.DaysOfWeek); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// NullStringDB returns nil for empty string to be used in DB queries.
func NullStringDB(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}
