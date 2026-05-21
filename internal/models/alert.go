package models

import "time"

type Alert struct {
	ID                int        `json:"id"`
	LampadaireID      *int       `json:"lampadaire_id,omitempty"`
	CabinetID         *int       `json:"cabinet_id,omitempty"`
	BasestationID     *int       `json:"basestation_id,omitempty"`
	CircuitID         *int       `json:"circuit_id,omitempty"`
	WorkOrderID       *int       `json:"work_order_id,omitempty"`
	SourceType        string     `json:"source_type,omitempty"`
	Type              string     `json:"type"`
	Severity          string     `json:"severity"`
	Message           string     `json:"message"`
	Status            string     `json:"status"`
	ProbableCause     string     `json:"probable_cause,omitempty"`
	RecommendedAction string     `json:"recommended_action,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	AcknowledgedAt    *time.Time `json:"acknowledged_at,omitempty"`
	ResolvedAt        *time.Time `json:"resolved_at,omitempty"`
	ClosedAt          *time.Time `json:"closed_at,omitempty"`
	Reference           string     `json:"reference,omitempty"`
	LCUID               *int       `json:"lcu_id,omitempty"`
	Zone                string     `json:"zone,omitempty"`
	MaintenanceRelated  bool       `json:"maintenance_related,omitempty"`
	MaintenanceWindowID *int       `json:"maintenance_window_id,omitempty"`
}
