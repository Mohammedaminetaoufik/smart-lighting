package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func insertWorkOrder(db *sql.DB, wo *WorkOrder) error {
	return db.QueryRow(`
		INSERT INTO work_orders (title, description, priority, status,
			lampadaire_id, cabinet_id, basestation_id, circuit_id,
			assigned_to, crew_type, due_date, probable_cause)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id, created_at, updated_at`,
		wo.Title, wo.Description,
		orDefault(wo.Priority, "medium"),
		orDefault(wo.Status, "created"),
		nullableInt(wo.LampadaireID), nullableInt(wo.CabinetID),
		nullableInt(wo.BasestationID), nullableInt(wo.CircuitID),
		nullableInt(wo.AssignedTo),
		orDefault(wo.CrewType, "lighting"),
		wo.DueDate,
		wo.ProbableCause,
	).Scan(&wo.ID, &wo.CreatedAt, &wo.UpdatedAt)
}

func listWorkOrders(db *sql.DB) ([]WorkOrder, error) {
	rows, err := db.Query(`
		SELECT id, title, description, priority, status,
			lampadaire_id, cabinet_id, basestation_id, circuit_id,
			assigned_to, crew_type, due_date, probable_cause, resolution_note,
			created_at, updated_at, closed_at
		FROM work_orders ORDER BY
			CASE priority WHEN 'urgent' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 ELSE 4 END,
			created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []WorkOrder
	for rows.Next() {
		if wo, err := scanWorkOrderRow(rows.Scan); err == nil {
			wo.SourceAlertIDs, _ = getWorkOrderAlertIDs(db, wo.ID)
			list = append(list, wo)
		}
	}
	return list, nil
}

func getWorkOrderByID(db *sql.DB, id int) (*WorkOrder, error) {
	row := db.QueryRow(`
		SELECT id, title, description, priority, status,
			lampadaire_id, cabinet_id, basestation_id, circuit_id,
			assigned_to, crew_type, due_date, probable_cause, resolution_note,
			created_at, updated_at, closed_at
		FROM work_orders WHERE id = $1`, id)
	wo, err := scanWorkOrderRow(row.Scan)
	if err != nil {
		return nil, err
	}
	wo.SourceAlertIDs, _ = getWorkOrderAlertIDs(db, wo.ID)
	return &wo, nil
}

func updateWorkOrderStatus(db *sql.DB, id int, status, note string) error {
	now := time.Now()
	if status == "closed" || status == "resolved" {
		_, err := db.Exec(`
			UPDATE work_orders SET status=$1, resolution_note=$2, closed_at=$3, updated_at=$3
			WHERE id=$4`, status, note, now, id)
		return err
	}
	_, err := db.Exec(`UPDATE work_orders SET status=$1, updated_at=$2 WHERE id=$3`, status, now, id)
	return err
}

func assignWorkOrder(db *sql.DB, id, userID int) error {
	now := time.Now()
	_, err := db.Exec(`UPDATE work_orders SET assigned_to=$1, status='assigned', updated_at=$2 WHERE id=$3`,
		userID, now, id)
	return err
}

func linkAlertsToWorkOrder(db *sql.DB, workOrderID int, alertIDs []int) error {
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

func getWorkOrderAlertIDs(db *sql.DB, workOrderID int) ([]int, error) {
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

func createWorkOrderFromAlerts(db *sql.DB, alertIDs []int, title, priority, crewType string) (*WorkOrder, error) {
	if len(alertIDs) == 0 {
		return nil, fmt.Errorf("no alert IDs provided")
	}
	wo := &WorkOrder{
		Title:    title,
		Priority: orDefault(priority, "medium"),
		CrewType: orDefault(crewType, "lighting"),
		Status:   "created",
	}
	// Inherit lampadaire from first alert if available
	var lampID sql.NullInt64
	db.QueryRow(`SELECT lampadaire_id FROM alerts WHERE id=$1`, alertIDs[0]).Scan(&lampID)
	if lampID.Valid {
		v := int(lampID.Int64)
		wo.LampadaireID = &v
	}
	if err := insertWorkOrder(db, wo); err != nil {
		return nil, err
	}
	_ = linkAlertsToWorkOrder(db, wo.ID, alertIDs)
	return wo, nil
}

func scanWorkOrderRow(scan func(...any) error) (WorkOrder, error) {
	var wo WorkOrder
	var lampID, cabID, bsID, circID, assignedTo sql.NullInt64
	var dueDate, closedAt sql.NullTime
	err := scan(
		&wo.ID, &wo.Title, &wo.Description, &wo.Priority, &wo.Status,
		&lampID, &cabID, &bsID, &circID,
		&assignedTo, &wo.CrewType, &dueDate, &wo.ProbableCause, &wo.ResolutionNote,
		&wo.CreatedAt, &wo.UpdatedAt, &closedAt,
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
	wo.CabinetID = nullInt(cabID)
	wo.BasestationID = nullInt(bsID)
	wo.CircuitID = nullInt(circID)
	wo.AssignedTo = nullInt(assignedTo)
	if dueDate.Valid {
		wo.DueDate = &dueDate.Time
	}
	if closedAt.Valid {
		wo.ClosedAt = &closedAt.Time
	}
	return wo, nil
}
