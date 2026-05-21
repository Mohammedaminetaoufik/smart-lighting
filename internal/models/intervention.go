package models

import "time"

type Intervention struct {
	ID             int        `json:"id"`
	WorkOrderID    *int       `json:"work_order_id,omitempty"`
	AlertID        *int       `json:"alert_id,omitempty"`
	LampadaireID   *int       `json:"lampadaire_id,omitempty"`
	AssignedTo     *int       `json:"assigned_to,omitempty"`
	TechnicianName string     `json:"technician_name,omitempty"`
	Title          string     `json:"title"`
	Description    string     `json:"description,omitempty"`
	ActionTaken    string     `json:"action_taken,omitempty"`
	Note           string     `json:"note,omitempty"`
	Priority       string     `json:"priority"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
	ClosedAt       *time.Time `json:"closed_at,omitempty"`
}
