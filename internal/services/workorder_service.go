package services

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"map-interactif/internal/models"
)

// AlertDiagnosis holds the result of DiagnoseAlert.
type AlertDiagnosis struct {
	ProbableCause       string
	RecommendedAction   string
	Priority            string
	TeamType            string
	EquipmentType       string
	ShouldCreateWorkOrder bool
	IsAutoCreate        bool // auto-create without user click
}

// DiagnoseAlert inspects an alert and returns a structured diagnosis.
// Pure function — no DB calls.
func DiagnoseAlert(a *models.Alert) AlertDiagnosis {
	d := AlertDiagnosis{
		EquipmentType: "lampadaire",
		TeamType:      "lighting",
		Priority:      "medium",
	}

	switch a.Type {
	case "lcu_offline", "lcu_unreachable":
		d.ProbableCause = "Passerelle LCU hors ligne — alimentation, réseau ou configuration IP/port"
		d.RecommendedAction = "Vérifier l'alimentation de la LCU, ping réseau, redémarrage si nécessaire"
		d.Priority = "urgent"
		d.TeamType = "network"
		d.EquipmentType = "lcu"
		d.ShouldCreateWorkOrder = true
		d.IsAutoCreate = true

	case "lampadaire_offline", "device_offline":
		d.ProbableCause = "Lampadaire hors ligne — contrôleur ou communication LCU défaillante"
		d.RecommendedAction = "Vérifier communication LCU et device_uid ; tester le contrôleur sur site"
		d.Priority = priorityFromSeverity(a.Severity)
		d.TeamType = "lighting"
		d.ShouldCreateWorkOrder = a.Severity == "critical"
		d.IsAutoCreate = a.Severity == "critical"

	case "controller_offline", "controller_lost":
		d.ProbableCause = "Contrôleur non joignable — signal faible ou panne hardware"
		d.RecommendedAction = "Tester la communication et la qualité du signal ; remplacer le module si nécessaire"
		d.Priority = "high"
		d.TeamType = "network"
		d.ShouldCreateWorkOrder = true
		d.IsAutoCreate = a.Severity == "critical"

	case "high_temperature", "driver_temperature":
		d.ProbableCause = "Surchauffe du driver LED ou du module — température anormalement élevée"
		d.RecommendedAction = "Réduire l'intensité à 50 %, vérifier la ventilation et l'état du driver"
		d.Priority = "urgent"
		d.TeamType = "lighting"
		d.ShouldCreateWorkOrder = true
		d.IsAutoCreate = a.Severity == "critical"

	case "abnormal_power", "overconsumption":
		d.ProbableCause = "Surconsommation ou driver défectueux — courant hors norme"
		d.RecommendedAction = "Vérifier le driver, la puissance nominale et la continuité du circuit"
		d.Priority = "high"
		d.TeamType = "electrical"
		d.ShouldCreateWorkOrder = true
		d.IsAutoCreate = a.Severity == "critical"

	case "low_signal", "weak_signal":
		d.ProbableCause = "Qualité du signal radio insuffisante — obstacles ou distance trop grande"
		d.RecommendedAction = "Repositionner l'antenne ou ajouter un répéteur réseau"
		d.Priority = "medium"
		d.TeamType = "network"
		d.ShouldCreateWorkOrder = false
		d.IsAutoCreate = false

	case "dimming_failed", "dimming_error":
		d.ProbableCause = "Échec de la commande de variateur — driver non joignable ou incompatible"
		d.RecommendedAction = "Tester le protocole de dimming ; vérifier la compatibilité DALI/0-10V"
		d.Priority = "high"
		d.TeamType = "lighting"
		d.ShouldCreateWorkOrder = true
		d.IsAutoCreate = a.Severity == "critical"

	case "door_open", "cabinet_door":
		d.ProbableCause = "Porte d'armoire ouverte — accès non autorisé ou oubli de fermeture"
		d.RecommendedAction = "Vérifier l'armoire sur site et sécuriser la fermeture"
		d.Priority = "high"
		d.TeamType = "electrical"
		d.EquipmentType = "cabinet"
		d.ShouldCreateWorkOrder = true
		d.IsAutoCreate = true

	case "power_failure", "circuit_fault":
		d.ProbableCause = "Défaut d'alimentation ou disjoncteur déclenché sur le circuit"
		d.RecommendedAction = "Contrôler les fusibles, le disjoncteur et la tension de phase"
		d.Priority = "urgent"
		d.TeamType = "electrical"
		d.EquipmentType = "cabinet"
		d.ShouldCreateWorkOrder = true
		d.IsAutoCreate = true

	case "commissioning_failed", "commissioning_error":
		d.ProbableCause = "Échec de la mise en service — configuration incomplète ou test de communication négatif"
		d.RecommendedAction = "Vérifier l'adresse réseau, le protocole DALI/0-10V et refaire les tests de commissioning"
		d.Priority = "high"
		d.TeamType = "lighting"
		d.EquipmentType = "lampadaire"
		d.ShouldCreateWorkOrder = true
		d.IsAutoCreate = false

	default:
		d.ProbableCause = "Anomalie détectée — analyse manuelle requise"
		d.RecommendedAction = "Consulter les logs système et les dernières télémesures"
		d.Priority = priorityFromSeverity(a.Severity)
		d.ShouldCreateWorkOrder = a.Severity == "critical"
		d.IsAutoCreate = a.Severity == "critical"
	}

	// Override: info alerts never create work orders
	if a.Severity == "info" {
		d.ShouldCreateWorkOrder = false
		d.IsAutoCreate = false
		d.Priority = "low"
	}

	return d
}

func priorityFromSeverity(severity string) string {
	switch severity {
	case "critical":
		return "urgent"
	case "major":
		return "high"
	case "warning":
		return "medium"
	default:
		return "low"
	}
}

// FindExistingOpenWorkOrderForAlert checks if an open work order already covers this alert.
// Returns (workOrderID, found, error).
// Rules (in order):
//  1. Alert already has a work_order_id set.
//  2. Same lampadaire_id + same alert type with an open WO.
//  3. Same lcu_id + same alert type with an open WO.
//  4. Same zone + same alert type with an open WO (group case).
func FindExistingOpenWorkOrderForAlert(db *sql.DB, a *models.Alert) (int, bool, error) {
	// 1. Alert already has a work order
	if a.WorkOrderID != nil && *a.WorkOrderID > 0 {
		return *a.WorkOrderID, true, nil
	}

	openStatuses := "('open','created','assigned','in_progress','waiting_parts')"

	// 2. Same lampadaire + alert type
	if a.LampadaireID != nil {
		var woID int
		err := db.QueryRow(`
			SELECT wo.id FROM work_orders wo
			JOIN work_order_alerts woa ON woa.work_order_id = wo.id
			JOIN alerts al ON al.id = woa.alert_id
			WHERE al.lampadaire_id = $1 AND al.type = $2
			  AND wo.status IN `+openStatuses+`
			ORDER BY wo.created_at DESC LIMIT 1`,
			*a.LampadaireID, a.Type).Scan(&woID)
		if err == nil {
			return woID, true, nil
		}
	}

	// 3. Same LCU + alert type
	if a.LCUID != nil {
		var woID int
		err := db.QueryRow(`
			SELECT id FROM work_orders
			WHERE lcu_id = $1 AND status IN `+openStatuses+`
			  AND (probable_cause LIKE $2 OR title LIKE $3)
			ORDER BY created_at DESC LIMIT 1`,
			*a.LCUID,
			"%"+a.Type+"%",
			"%"+a.Type+"%",
		).Scan(&woID)
		if err == nil {
			return woID, true, nil
		}
	}

	// 4. Same zone + alert type (group issue)
	if a.Zone != "" {
		var woID int
		err := db.QueryRow(`
			SELECT id FROM work_orders
			WHERE zone = $1 AND status IN `+openStatuses+`
			  AND (probable_cause LIKE $2 OR title LIKE $3)
			ORDER BY created_at DESC LIMIT 1`,
			a.Zone,
			"%"+a.Type+"%",
			"%"+a.Type+"%",
		).Scan(&woID)
		if err == nil {
			return woID, true, nil
		}
	}

	return 0, false, nil
}

// buildWorkOrderTitle builds a human-readable title based on alert type and context.
func buildWorkOrderTitle(a *models.Alert, d AlertDiagnosis) string {
	ref := a.Reference
	if ref == "" {
		if a.LampadaireID != nil {
			ref = fmt.Sprintf("Lampadaire #%d", *a.LampadaireID)
		} else if a.LCUID != nil {
			ref = fmt.Sprintf("LCU #%d", *a.LCUID)
		} else {
			ref = "Équipement"
		}
	}

	typeLabel := strings.ReplaceAll(a.Type, "_", " ")
	typeLabel = strings.Title(typeLabel)

	switch a.Type {
	case "lcu_offline", "lcu_unreachable":
		return fmt.Sprintf("Vérifier LCU hors ligne — %s", ref)
	case "lampadaire_offline", "device_offline":
		return fmt.Sprintf("Lampadaire hors ligne à réparer — %s", ref)
	case "controller_offline", "controller_lost":
		return fmt.Sprintf("Contrôleur non joignable — %s", ref)
	case "high_temperature", "driver_temperature":
		return fmt.Sprintf("Surchauffe driver — %s", ref)
	case "abnormal_power", "overconsumption":
		return fmt.Sprintf("Anomalie consommation — %s", ref)
	case "dimming_failed":
		return fmt.Sprintf("Dimming en échec — %s", ref)
	case "door_open", "cabinet_door":
		return fmt.Sprintf("Porte armoire ouverte — %s", ref)
	case "power_failure", "circuit_fault":
		return fmt.Sprintf("Défaut alimentation — %s", ref)
	case "commissioning_failed", "commissioning_error":
		return fmt.Sprintf("Échec mise en service — %s", ref)
	default:
		return fmt.Sprintf("%s — %s", typeLabel, ref)
	}
}

// CreateWorkOrderFromAlert creates (or returns existing) work order for the given alert.
// Returns (workOrder, alreadyExisted, error).
func CreateWorkOrderFromAlert(db *sql.DB, alertID int) (*models.WorkOrder, bool, error) {
	// Load the alert with zone + lcu context
	var a models.Alert
	var lampID, lcuID, woID sql.NullInt64
	var zone, ref sql.NullString
	err := db.QueryRow(`
		SELECT a.id, a.lampadaire_id, a.type, a.severity, a.message, a.status,
		       a.work_order_id, a.source_type,
		       l.zone, l.reference, l.lcu_id
		FROM alerts a
		LEFT JOIN lampadaires l ON a.lampadaire_id = l.id
		WHERE a.id = $1`, alertID).
		Scan(&a.ID, &lampID, &a.Type, &a.Severity, &a.Message, &a.Status,
			&woID, &a.SourceType, &zone, &ref, &lcuID)
	if err != nil {
		return nil, false, fmt.Errorf("alert %d not found: %w", alertID, err)
	}
	if lampID.Valid {
		v := int(lampID.Int64)
		a.LampadaireID = &v
	}
	if lcuID.Valid {
		v := int(lcuID.Int64)
		a.LCUID = &v
	}
	if zone.Valid {
		a.Zone = zone.String
	}
	if ref.Valid {
		a.Reference = ref.String
	}
	if woID.Valid {
		v := int(woID.Int64)
		a.WorkOrderID = &v
	}

	// Check for existing open work order
	existingID, found, err := FindExistingOpenWorkOrderForAlert(db, &a)
	if err != nil {
		return nil, false, err
	}
	if found {
		// Link this alert to the existing WO and return it
		_, _ = db.Exec(`INSERT INTO work_order_alerts (work_order_id, alert_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, existingID, alertID)
		_, _ = db.Exec(`UPDATE alerts SET work_order_id=$1 WHERE id=$2`, existingID, alertID)
		_, _ = db.Exec(`UPDATE work_orders SET repeat_count=repeat_count+1, updated_at=NOW() WHERE id=$1`, existingID)
		wo, err := GetWorkOrderByID(db, existingID)
		return wo, true, err
	}

	// Diagnose and create new work order
	d := DiagnoseAlert(&a)
	title := buildWorkOrderTitle(&a, d)

	var newWO models.WorkOrder
	now := time.Now()
	err = db.QueryRow(`
		INSERT INTO work_orders (
			title, description, priority, status,
			source_type, source_alert_id,
			lampadaire_id, lcu_id, zone,
			equipment_type, equipment_reference,
			crew_type, team_type,
			probable_cause, recommended_action,
			created_at, updated_at
		) VALUES ($1,$2,$3,'open','alert',$4,$5,$6,$7,$8,$9,'lighting',$10,$11,$12,$13,$14)
		RETURNING id, created_at, updated_at`,
		title,
		a.Message,
		d.Priority,
		alertID,
		NullableIntVal(a.LampadaireID),
		NullableIntVal(a.LCUID),
		nullStr(a.Zone),
		d.EquipmentType,
		nullStr(a.Reference),
		d.TeamType,
		d.ProbableCause,
		d.RecommendedAction,
		now, now,
	).Scan(&newWO.ID, &newWO.CreatedAt, &newWO.UpdatedAt)
	if err != nil {
		return nil, false, fmt.Errorf("insert work_order: %w", err)
	}

	newWO.Title = title
	newWO.Description = a.Message
	newWO.Priority = d.Priority
	newWO.Status = "open"
	newWO.SourceType = "alert"
	newWO.EquipmentType = d.EquipmentType
	newWO.TeamType = d.TeamType
	newWO.ProbableCause = d.ProbableCause
	newWO.RecommendedAction = d.RecommendedAction
	newWO.SourceAlertID = &alertID
	newWO.LampadaireID = a.LampadaireID
	newWO.LCUID = a.LCUID
	newWO.Zone = a.Zone

	// Link alert to work order
	_, _ = db.Exec(`INSERT INTO work_order_alerts (work_order_id, alert_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, newWO.ID, alertID)
	_, _ = db.Exec(`UPDATE alerts SET work_order_id=$1 WHERE id=$2`, newWO.ID, alertID)

	return &newWO, false, nil
}

// AutoCreateWorkOrderIfNeeded is called after an alert is created.
// It creates a work order automatically only for critical cases.
// Returns (workOrder, created, error).
func AutoCreateWorkOrderIfNeeded(db *sql.DB, a *models.Alert) (*models.WorkOrder, bool, error) {
	d := DiagnoseAlert(a)
	if !d.IsAutoCreate {
		return nil, false, nil
	}

	// Check if a repeated alert: count open alerts of same type for same equipment
	var count int
	if a.LampadaireID != nil {
		_ = db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE lampadaire_id=$1 AND type=$2 AND status='open'`,
			*a.LampadaireID, a.Type).Scan(&count)
	}
	// If more than 3 of the same type, definitely create
	if !d.IsAutoCreate && count < 3 {
		return nil, false, nil
	}

	wo, existed, err := CreateWorkOrderFromAlert(db, a.ID)
	if err != nil {
		return nil, false, err
	}
	return wo, !existed, nil
}

// GetWorkOrderByID fetches a single work order. Duplicated here to avoid import cycles.
func GetWorkOrderByID(db *sql.DB, id int) (*models.WorkOrder, error) {
	var wo models.WorkOrder
	var lampID, lcuID, cabID, bsID, circID, assignedTo, srcAlertID sql.NullInt64
	var dueDate, closedAt, resolvedAt sql.NullTime
	var zone, eqType, eqRef, teamType, srcType, recommendedAction, assigneeName sql.NullString
	err := db.QueryRow(`
		SELECT wo.id, wo.title, wo.description, wo.priority, wo.status,
		       wo.source_type, wo.source_alert_id,
		       wo.lampadaire_id, wo.lcu_id, wo.zone,
		       wo.cabinet_id, wo.basestation_id, wo.circuit_id,
		       wo.equipment_type, wo.equipment_reference,
		       wo.assigned_to, COALESCE(u.full_name,'') as assignee_name,
		       wo.crew_type, wo.team_type,
		       wo.due_date, wo.probable_cause, wo.recommended_action, wo.resolution_note,
		       wo.repeat_count, wo.created_at, wo.updated_at, wo.resolved_at, wo.closed_at
		FROM work_orders wo
		LEFT JOIN users u ON u.id = wo.assigned_to
		WHERE wo.id = $1`, id).
		Scan(&wo.ID, &wo.Title, &wo.Description, &wo.Priority, &wo.Status,
			&srcType, &srcAlertID,
			&lampID, &lcuID, &zone,
			&cabID, &bsID, &circID,
			&eqType, &eqRef,
			&assignedTo, &assigneeName,
			&wo.CrewType, &teamType,
			&dueDate, &wo.ProbableCause, &recommendedAction, &wo.ResolutionNote,
			&wo.RepeatCount, &wo.CreatedAt, &wo.UpdatedAt, &resolvedAt, &closedAt)
	if err != nil {
		return nil, err
	}
	nullInt := func(n sql.NullInt64) *int {
		if n.Valid {
			v := int(n.Int64)
			return &v
		}
		return nil
	}
	wo.LampadaireID = nullInt(lampID)
	wo.LCUID = nullInt(lcuID)
	wo.CabinetID = nullInt(cabID)
	wo.BasestationID = nullInt(bsID)
	wo.CircuitID = nullInt(circID)
	wo.AssignedTo = nullInt(assignedTo)
	wo.SourceAlertID = nullInt(srcAlertID)
	if srcType.Valid {
		wo.SourceType = srcType.String
	}
	if zone.Valid {
		wo.Zone = zone.String
	}
	if eqType.Valid {
		wo.EquipmentType = eqType.String
	}
	if eqRef.Valid {
		wo.EquipmentReference = eqRef.String
	}
	if teamType.Valid {
		wo.TeamType = teamType.String
	}
	if recommendedAction.Valid {
		wo.RecommendedAction = recommendedAction.String
	}
	if assigneeName.Valid {
		wo.AssigneeName = assigneeName.String
	}
	if dueDate.Valid {
		wo.DueDate = &dueDate.Time
	}
	if resolvedAt.Valid {
		wo.ResolvedAt = &resolvedAt.Time
	}
	if closedAt.Valid {
		wo.ClosedAt = &closedAt.Time
	}
	return &wo, nil
}

// NullableIntVal converts *int to sql.NullInt64.
func NullableIntVal(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
