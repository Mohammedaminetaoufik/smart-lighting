package models

import "time"

type Intervention struct {
	ID           int        `json:"id"`
	AlertID      *int       `json:"alert_id,omitempty"`
	LampadaireID *int       `json:"lampadaire_id,omitempty"`
	AssignedTo   *int       `json:"assigned_to,omitempty"`
	Title        string     `json:"title"`
	Description  string     `json:"description,omitempty"`
	Priority     string     `json:"priority"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ClosedAt     *time.Time `json:"closed_at,omitempty"`
}
