package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"map-interactif/internal/models"
	"map-interactif/internal/utils"
)

// ─── SQL fragment shared by ListWorkOrders and GetWorkOrderByID ──────────────

const woSelectCols = `
	wo.id, wo.title, COALESCE(wo.description,''), wo.priority, wo.status,
	COALESCE(wo.source_type,'manual'), wo.source_alert_id,
	wo.lampadaire_id, wo.lcu_id, wo.zone,
	wo.cabinet_id, wo.basestation_id, wo.circuit_id,
	COALESCE(wo.equipment_type,'lampadaire'), wo.equipment_reference,
	wo.assigned_to, COALESCE(wo.assigned_to_name,''), wo.technician_id,
	COALESCE(u.full_name,'') as assignee_name,
	COALESCE(wo.crew_type,'lighting'), COALESCE(wo.team_type,'lighting'),
	wo.due_date,
	COALESCE(wo.probable_cause,''), COALESCE(wo.recommended_action,''),
	COALESCE(wo.resolution_note,''), COALESCE(wo.resolution_type,''), COALESCE(wo.closing_note,''),
	COALESCE(wo.repeat_count,1),
	wo.created_by, wo.closed_by,
	wo.created_at, wo.updated_at,
	wo.accepted_at, wo.started_at, wo.resolved_at, wo.cancelled_at, wo.closed_at`

// ─── Insert ──────────────────────────────────────────────────────────────────

// InsertWorkOrder inserts a new work order.
func InsertWorkOrder(db *sql.DB, wo *models.WorkOrder) error {
	return db.QueryRow(`
		INSERT INTO work_orders (title, description, priority, status,
			source_type, source_alert_id,
			lampadaire_id, lcu_id, zone,
			cabinet_id, basestation_id, circuit_id,
			equipment_type, equipment_reference,
			assigned_to, crew_type, team_type,
			due_date, probable_cause, recommended_action,
			created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
		RETURNING id, created_at, updated_at`,
		wo.Title, wo.Description,
		utils.OrDefault(wo.Priority, "medium"),
		utils.OrDefault(wo.Status, "open"),
		utils.OrDefault(wo.SourceType, "manual"),
		NullableInt(wo.SourceAlertID),
		NullableInt(wo.LampadaireID), NullableInt(wo.LCUID),
		nullableStr(wo.Zone),
		NullableInt(wo.CabinetID),
		NullableInt(wo.BasestationID), NullableInt(wo.CircuitID),
		utils.OrDefault(wo.EquipmentType, "lampadaire"),
		nullableStr(wo.EquipmentReference),
		NullableInt(wo.AssignedTo),
		utils.OrDefault(wo.CrewType, "lighting"),
		utils.OrDefault(wo.TeamType, "lighting"),
		wo.DueDate,
		wo.ProbableCause, wo.RecommendedAction,
		NullableInt(wo.CreatedBy),
	).Scan(&wo.ID, &wo.CreatedAt, &wo.UpdatedAt)
}

func nullableStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// ─── Read ─────────────────────────────────────────────────────────────────────

// ListWorkOrders returns all work orders ordered by priority then recency.
func ListWorkOrders(db *sql.DB) ([]models.WorkOrder, error) {
	rows, err := db.Query(`
		SELECT ` + woSelectCols + `
		FROM work_orders wo
		LEFT JOIN users u ON u.id = wo.assigned_to
		ORDER BY
			CASE wo.priority WHEN 'urgent' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 ELSE 4 END,
			wo.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.WorkOrder
	for rows.Next() {
		if wo, err := scanWorkOrderRow(rows.Scan); err == nil {
			wo.SourceAlertIDs, _ = GetWorkOrderAlertIDs(db, wo.ID)
			list = append(list, wo)
		}
	}
	return list, nil
}

// GetWorkOrderByID fetches a single work order by ID.
func GetWorkOrderByID(db *sql.DB, id int) (*models.WorkOrder, error) {
	row := db.QueryRow(`
		SELECT `+woSelectCols+`
		FROM work_orders wo
		LEFT JOIN users u ON u.id = wo.assigned_to
		WHERE wo.id = $1`, id)
	wo, err := scanWorkOrderRow(row.Scan)
	if err != nil {
		return nil, err
	}
	wo.SourceAlertIDs, _ = GetWorkOrderAlertIDs(db, wo.ID)
	return &wo, nil
}

// GetOpenWorkOrders returns all work orders in status open or accepted.
func GetOpenWorkOrders(db *sql.DB) ([]models.WorkOrder, error) {
	rows, err := db.Query(`
		SELECT `+woSelectCols+`
		FROM work_orders wo
		LEFT JOIN users u ON u.id = wo.assigned_to
		WHERE wo.status IN ('open','accepted')
		ORDER BY
			CASE wo.priority WHEN 'urgent' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 ELSE 4 END,
			wo.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.WorkOrder
	for rows.Next() {
		if wo, err := scanWorkOrderRow(rows.Scan); err == nil {
			list = append(list, wo)
		}
	}
	return list, nil
}

// ─── Status transitions ───────────────────────────────────────────────────────

// UpdateWorkOrderStatus updates status and the matching timestamp.
// Also inserts a log entry.
func UpdateWorkOrderStatus(db *sql.DB, id int, status, note string) error {
	now := time.Now()

	var q string
	switch status {
	case "accepted":
		q = `UPDATE work_orders SET status=$1, accepted_at=$2, updated_at=$2 WHERE id=$3`
	case "in_progress":
		q = `UPDATE work_orders SET status=$1, started_at=$2, updated_at=$2 WHERE id=$3`
	case "resolved":
		q = `UPDATE work_orders SET status=$1, resolution_note=$4, resolved_at=$2, updated_at=$2 WHERE id=$3`
	case "cancelled":
		q = `UPDATE work_orders SET status=$1, cancelled_at=$2, closed_at=$2, resolution_note=$4, updated_at=$2 WHERE id=$3`
	case "closed":
		q = `UPDATE work_orders SET status=$1, closed_at=$2, closing_note=$4, updated_at=$2 WHERE id=$3`
	default:
		q = `UPDATE work_orders SET status=$1, updated_at=$2 WHERE id=$3`
	}

	var err error
	if strings.Contains(q, "$4") {
		_, err = db.Exec(q, status, now, id, note)
	} else {
		_, err = db.Exec(q, status, now, id)
	}
	if err != nil {
		return err
	}

	// Fetch old status for log
	var oldStatus string
	_ = db.QueryRow(`SELECT status FROM work_orders WHERE id=$1`, id).Scan(&oldStatus)

	action := statusToLogAction(status)
	_ = InsertWorkOrderLog(db, &models.WorkOrderLog{
		WorkOrderID: id,
		Action:      action,
		Note:        note,
		OldStatus:   oldStatus,
		NewStatus:   status,
	})
	return nil
}

func statusToLogAction(status string) string {
	switch status {
	case "accepted":
		return "accepted"
	case "in_progress":
		return "started"
	case "resolved":
		return "resolved"
	case "closed":
		return "closed"
	case "cancelled":
		return "cancelled"
	case "open":
		return "reopened"
	default:
		return "updated"
	}
}

// AcceptWorkOrder sets status to accepted and records the technician.
// Fails if status != open.
func AcceptWorkOrder(db *sql.DB, id, technicianID int, technicianName string) error {
	now := time.Now()
	res, err := db.Exec(`
		UPDATE work_orders
		SET status='accepted', technician_id=$1, assigned_to_name=$2, accepted_at=$3, updated_at=$3
		WHERE id=$4 AND status='open'`,
		technicianID, technicianName, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("work order is not open or does not exist")
	}
	_ = InsertWorkOrderLog(db, &models.WorkOrderLog{
		WorkOrderID: id,
		UserName:    technicianName,
		Action:      "accepted",
		OldStatus:   "open",
		NewStatus:   "accepted",
	})
	return nil
}

// AssignWorkOrder assigns a work order to a user.
func AssignWorkOrder(db *sql.DB, id, userID int) error {
	now := time.Now()
	_, err := db.Exec(`UPDATE work_orders SET assigned_to=$1, status='accepted', updated_at=$2 WHERE id=$3`,
		userID, now, id)
	if err != nil {
		return err
	}
	_ = InsertWorkOrderLog(db, &models.WorkOrderLog{
		WorkOrderID: id,
		Action:      "accepted",
		OldStatus:   "open",
		NewStatus:   "accepted",
	})
	return nil
}

// ReopenWorkOrder resets a resolved work order to open.
func ReopenWorkOrder(db *sql.DB, id int, note string) error {
	res, err := db.Exec(`
		UPDATE work_orders
		SET status='open', resolved_at=NULL, resolution_note='', updated_at=NOW()
		WHERE id=$1 AND status='resolved'`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("work order cannot be reopened (status must be resolved)")
	}
	_ = InsertWorkOrderLog(db, &models.WorkOrderLog{
		WorkOrderID: id,
		Action:      "reopened",
		Note:        note,
		OldStatus:   "resolved",
		NewStatus:   "open",
	})
	return nil
}

// ─── Logs ─────────────────────────────────────────────────────────────────────

// InsertWorkOrderLog inserts an audit entry.
func InsertWorkOrderLog(db *sql.DB, l *models.WorkOrderLog) error {
	_, err := db.Exec(`
		INSERT INTO work_order_logs (work_order_id, user_id, user_name, role, action, note, old_status, new_status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		l.WorkOrderID, NullableInt(l.UserID), nullableStr(l.UserName),
		nullableStr(l.Role), l.Action, nullableStr(l.Note),
		nullableStr(l.OldStatus), nullableStr(l.NewStatus))
	return err
}

// GetWorkOrderLogs returns all log entries for a work order.
func GetWorkOrderLogs(db *sql.DB, workOrderID int) ([]models.WorkOrderLog, error) {
	rows, err := db.Query(`
		SELECT id, work_order_id, user_id, COALESCE(user_name,''), COALESCE(role,''),
		       action, COALESCE(note,''), COALESCE(old_status,''), COALESCE(new_status,''), created_at
		FROM work_order_logs
		WHERE work_order_id=$1
		ORDER BY created_at ASC`, workOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.WorkOrderLog
	for rows.Next() {
		var l models.WorkOrderLog
		var uid sql.NullInt64
		if err := rows.Scan(&l.ID, &l.WorkOrderID, &uid, &l.UserName, &l.Role,
			&l.Action, &l.Note, &l.OldStatus, &l.NewStatus, &l.CreatedAt); err != nil {
			continue
		}
		if uid.Valid {
			v := int(uid.Int64)
			l.UserID = &v
		}
		list = append(list, l)
	}
	return list, nil
}

// AddWorkOrderNote inserts a note log entry without changing status.
func AddWorkOrderNote(db *sql.DB, workOrderID int, note, userName string) error {
	if note == "" {
		return fmt.Errorf("note cannot be empty")
	}
	_, err := db.Exec(`UPDATE work_orders SET updated_at=NOW() WHERE id=$1`, workOrderID)
	if err != nil {
		return err
	}
	return InsertWorkOrderLog(db, &models.WorkOrderLog{
		WorkOrderID: workOrderID,
		UserName:    userName,
		Action:      "note_added",
		Note:        note,
	})
}

// ─── Links ────────────────────────────────────────────────────────────────────

// LinkAlertsToWorkOrder links alerts to a work order (ignores duplicates).
func LinkAlertsToWorkOrder(db *sql.DB, workOrderID int, alertIDs []int) error {
	if len(alertIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(alertIDs))
	args := make([]any, len(alertIDs)+1)
	args[0] = workOrderID
	for i, aid := range alertIDs {
		placeholders[i] = fmt.Sprintf("($1, $%d)", i+2)
		args[i+1] = aid
	}
	_, err := db.Exec(
		`INSERT INTO work_order_alerts (work_order_id, alert_id) VALUES `+
			strings.Join(placeholders, ",")+` ON CONFLICT DO NOTHING`,
		args...,
	)
	return err
}

// GetWorkOrderAlertIDs returns the alert IDs linked to a work order.
func GetWorkOrderAlertIDs(db *sql.DB, workOrderID int) ([]int, error) {
	rows, err := db.Query(`SELECT alert_id FROM work_order_alerts WHERE work_order_id=$1`, workOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// GetWorkOrderLinkedAlerts returns full alert records linked to a work order.
func GetWorkOrderLinkedAlerts(db *sql.DB, workOrderID int) ([]models.Alert, error) {
	rows, err := db.Query(`
		SELECT a.id, a.lampadaire_id, a.type, a.severity, a.message, a.status,
		       a.created_at, a.resolved_at,
		       COALESCE(a.probable_cause,''), COALESCE(a.recommended_action,''),
		       COALESCE(l.reference,'')
		FROM alerts a
		JOIN work_order_alerts woa ON woa.alert_id = a.id
		LEFT JOIN lampadaires l ON a.lampadaire_id = l.id
		WHERE woa.work_order_id = $1
		ORDER BY a.created_at DESC`, workOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.Alert
	for rows.Next() {
		var a models.Alert
		var lid sql.NullInt64
		var resolved sql.NullTime
		if err := rows.Scan(&a.ID, &lid, &a.Type, &a.Severity, &a.Message, &a.Status,
			&a.CreatedAt, &resolved, &a.ProbableCause, &a.RecommendedAction, &a.Reference); err != nil {
			continue
		}
		if lid.Valid {
			v := int(lid.Int64)
			a.LampadaireID = &v
		}
		if resolved.Valid {
			a.ResolvedAt = &resolved.Time
		}
		list = append(list, a)
	}
	return list, nil
}

// GetWorkOrderInterventions returns interventions linked to a work order.
func GetWorkOrderInterventions(db *sql.DB, workOrderID int) ([]models.Intervention, error) {
	rows, err := db.Query(`
		SELECT i.id, i.work_order_id, i.alert_id, i.lampadaire_id, i.assigned_to,
		       COALESCE(i.technician_name,''), i.title, COALESCE(i.description,''),
		       COALESCE(i.action_taken,''), COALESCE(i.note,''),
		       i.priority, i.status, i.created_at, i.updated_at, i.resolved_at, i.closed_at
		FROM interventions i
		WHERE i.work_order_id = $1
		ORDER BY i.created_at DESC`, workOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.Intervention
	for rows.Next() {
		var iv models.Intervention
		var woID, alertID, lampID, assignedTo sql.NullInt64
		var resolvedAt, closedAt sql.NullTime
		if err := rows.Scan(&iv.ID, &woID, &alertID, &lampID, &assignedTo,
			&iv.TechnicianName, &iv.Title, &iv.Description,
			&iv.ActionTaken, &iv.Note,
			&iv.Priority, &iv.Status, &iv.CreatedAt, &iv.UpdatedAt, &resolvedAt, &closedAt); err != nil {
			continue
		}
		if woID.Valid {
			v := int(woID.Int64)
			iv.WorkOrderID = &v
		}
		if alertID.Valid {
			v := int(alertID.Int64)
			iv.AlertID = &v
		}
		if lampID.Valid {
			v := int(lampID.Int64)
			iv.LampadaireID = &v
		}
		if assignedTo.Valid {
			v := int(assignedTo.Int64)
			iv.AssignedTo = &v
		}
		if resolvedAt.Valid {
			iv.ResolvedAt = &resolvedAt.Time
		}
		if closedAt.Valid {
			iv.ClosedAt = &closedAt.Time
		}
		list = append(list, iv)
	}
	return list, nil
}

// ─── Bulk helpers ─────────────────────────────────────────────────────────────

// CreateWorkOrderFromAlerts creates a work order manually from a list of alert IDs.
func CreateWorkOrderFromAlerts(db *sql.DB, alertIDs []int, title, priority, crewType string) (*models.WorkOrder, error) {
	if len(alertIDs) == 0 {
		return nil, fmt.Errorf("no alert IDs provided")
	}
	wo := &models.WorkOrder{
		Title:      title,
		Priority:   utils.OrDefault(priority, "medium"),
		CrewType:   utils.OrDefault(crewType, "lighting"),
		TeamType:   utils.OrDefault(crewType, "lighting"),
		SourceType: "alert",
		Status:     "open",
	}
	var lampID sql.NullInt64
	db.QueryRow(`SELECT lampadaire_id FROM alerts WHERE id=$1`, alertIDs[0]).Scan(&lampID)
	if lampID.Valid {
		v := int(lampID.Int64)
		wo.LampadaireID = &v
	}
	if err := InsertWorkOrder(db, wo); err != nil {
		return nil, err
	}
	_ = LinkAlertsToWorkOrder(db, wo.ID, alertIDs)
	for _, aid := range alertIDs {
		_, _ = db.Exec(`UPDATE alerts SET work_order_id=$1 WHERE id=$2 AND work_order_id IS NULL`, wo.ID, aid)
	}
	_ = InsertWorkOrderLog(db, &models.WorkOrderLog{
		WorkOrderID: wo.ID,
		Action:      "created",
		NewStatus:   "open",
		Note:        fmt.Sprintf("Créé depuis %d alerte(s)", len(alertIDs)),
	})
	return wo, nil
}

// ─── Scan helper ──────────────────────────────────────────────────────────────

func scanWorkOrderRow(scan func(...any) error) (models.WorkOrder, error) {
	var wo models.WorkOrder
	var lampID, lcuID, cabID, bsID, circID, assignedTo, srcAlertID sql.NullInt64
	var technicianID, createdBy, closedBy sql.NullInt64
	var dueDate, closedAt, resolvedAt, acceptedAt, startedAt, cancelledAt sql.NullTime
	var zone, eqType, eqRef, teamType, srcType, recommendedAction sql.NullString
	var assignedToName, assigneeName, resType, closingNote sql.NullString
	err := scan(
		&wo.ID, &wo.Title, &wo.Description, &wo.Priority, &wo.Status,
		&srcType, &srcAlertID,
		&lampID, &lcuID, &zone,
		&cabID, &bsID, &circID,
		&eqType, &eqRef,
		&assignedTo, &assignedToName, &technicianID,
		&assigneeName,
		&wo.CrewType, &teamType,
		&dueDate,
		&wo.ProbableCause, &recommendedAction,
		&wo.ResolutionNote, &resType, &closingNote,
		&wo.RepeatCount,
		&createdBy, &closedBy,
		&wo.CreatedAt, &wo.UpdatedAt,
		&acceptedAt, &startedAt, &resolvedAt, &cancelledAt, &closedAt,
	)
	if err != nil {
		return wo, err
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
	wo.TechnicianID = nullInt(technicianID)
	wo.CreatedBy = nullInt(createdBy)
	wo.ClosedBy = nullInt(closedBy)
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
	if assignedToName.Valid {
		wo.AssignedToName = assignedToName.String
	}
	if assigneeName.Valid {
		wo.AssigneeName = assigneeName.String
	}
	if resType.Valid {
		wo.ResolutionType = resType.String
	}
	if closingNote.Valid {
		wo.ClosingNote = closingNote.String
	}
	if dueDate.Valid {
		wo.DueDate = &dueDate.Time
	}
	if acceptedAt.Valid {
		wo.AcceptedAt = &acceptedAt.Time
	}
	if startedAt.Valid {
		wo.StartedAt = &startedAt.Time
	}
	if resolvedAt.Valid {
		wo.ResolvedAt = &resolvedAt.Time
	}
	if cancelledAt.Valid {
		wo.CancelledAt = &cancelledAt.Time
	}
	if closedAt.Valid {
		wo.ClosedAt = &closedAt.Time
	}
	return wo, nil
}
