package services

import (
	"fmt"

	"map-interactif/internal/models"
)

// DiagnosticResult describes detected issues on a lampadaire.
type DiagnosticResult struct {
	HasIssue bool     `json:"has_issue"`
	Severity string   `json:"severity"`
	Issues   []string `json:"issues"`
}

// DiagnoseLampadaireIssue inspects embedded controller state and operational
// status to produce a diagnostic summary. Pure function — no DB calls.
func DiagnoseLampadaireIssue(l *models.Lampadaire) DiagnosticResult {
	var issues []string
	severity := "info"

	if l.ControllerStatus == "lost" {
		issues = append(issues, "Perte communication contrôleur")
		severity = "critical"
	}

	if l.Etat == "offline" && l.ControllerStatus == "ok" {
		issues = append(issues, "Lampadaire offline malgré contrôleur actif — vérifier le driver/pilote")
		if severity != "critical" {
			severity = "warning"
		}
	}

	if l.ControllerSignalQuality != nil && *l.ControllerSignalQuality < 30 {
		issues = append(issues, fmt.Sprintf("Signal contrôleur faible (%d%%)", *l.ControllerSignalQuality))
		if severity == "info" {
			severity = "warning"
		}
	}

	if l.DiscoveredByLCU && l.CommissioningStatus == "discovered" {
		issues = append(issues, "En attente de localisation GPS — aucune coordonnée disponible")
	}

	if !l.DimmingEnabled && l.CommissioningStatus == "commissioned" {
		issues = append(issues, "Dimming désactivé sur un équipement commissionné")
		if severity == "info" {
			severity = "warning"
		}
	}

	return DiagnosticResult{
		HasIssue: len(issues) > 0,
		Severity: severity,
		Issues:   issues,
	}
}
