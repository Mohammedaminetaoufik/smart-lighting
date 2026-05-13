package models

import "time"

// CalculatorDecision represents an intelligent calculator recommendation.
type CalculatorDecision struct {
	ID                   int       `json:"id"`
	LampadaireID         int       `json:"lampadaire_id"`
	RecommendedIntensity int       `json:"recommended_intensity"`
	DecisionReason       string    `json:"decision_reason"`
	Confidence           float64   `json:"confidence"`
	Applied              bool      `json:"applied"`
	RuleName             string    `json:"rule_name,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}
