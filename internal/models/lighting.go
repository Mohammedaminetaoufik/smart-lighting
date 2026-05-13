package models

import "time"

// LightingGroup represents a logical collection of lampadaires.
type LightingGroup struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Zone        string    `json:"zone,omitempty"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// LightingProfile represents a dimming strategy for a zone or group.
type LightingProfile struct {
	ID          int                       `json:"id"`
	Name        string                    `json:"name"`
	Description string                    `json:"description,omitempty"`
	TargetType  string                    `json:"target_type"`  // zone, group, lcu
	TargetValue string                    `json:"target_value"` // e.g. "Zone A" or GroupID
	Enabled     bool                      `json:"enabled"`
	Schedules   []LightingProfileSchedule `json:"schedules,omitempty"`
	CreatedAt   time.Time                 `json:"created_at"`
	UpdatedAt   time.Time                 `json:"updated_at"`
}

// LightingProfileSchedule represents a specific timing within a lighting profile.
type LightingProfileSchedule struct {
	ID         int       `json:"id"`
	ProfileID  int       `json:"profile_id"`
	StartTime  string    `json:"start_time"` // HH:MM
	EndTime    string    `json:"end_time"`   // HH:MM
	Intensity  int       `json:"intensity"`
	DaysOfWeek string    `json:"days_of_week,omitempty"` // 1,2,3,4,5,6,7
	CreatedAt  time.Time `json:"created_at"`
}

// LightingGroupMember is a member of a lighting group (placeholder for future use).
type LightingGroupMember struct {
	GroupID      int `json:"group_id"`
	LampadaireID int `json:"lampadaire_id"`
}
