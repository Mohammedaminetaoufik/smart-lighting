package models

import "time"

// DimmingCommand represents a dimming order sent to a lampadaire.
type DimmingCommand struct {
	ID           int        `json:"id"`
	LampadaireID int        `json:"lampadaire_id"`
	Source       string     `json:"source"`
	OldIntensity *int       `json:"old_intensity,omitempty"`
	NewIntensity int        `json:"new_intensity"`
	Reason       string     `json:"reason,omitempty"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	AppliedAt    *time.Time `json:"applied_at,omitempty"`
}
