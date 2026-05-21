package repository

import (
	"context"
	"database/sql"
	"fmt"

	"map-interactif/internal/models"
)

// CreateAlertIfNotExists creates an alert only if no open alert of the same type exists for that lampadaire.
func CreateAlertIfNotExists(ctx context.Context, db DBExecutor, lampadaireID int, alertType string, severity string, message string) (*models.Alert, error) {
	var count int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM alerts WHERE lampadaire_id=$1 AND type=$2 AND status='open'`,
		lampadaireID, alertType).Scan(&count)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, nil
	}

	var alert models.Alert
	err = db.QueryRowContext(ctx, `
		INSERT INTO alerts (lampadaire_id, type, severity, message)
		VALUES ($1, $2, $3, $4)
		RETURNING id, lampadaire_id, type, severity, message, status, created_at
	`, lampadaireID, alertType, severity, message).Scan(
		&alert.ID, &alert.LampadaireID, &alert.Type, &alert.Severity,
		&alert.Message, &alert.Status, &alert.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &alert, nil
}

// ResolveOpenAlert marks an open alert as resolved.
func ResolveOpenAlert(ctx context.Context, db DBExecutor, lampadaireID int, alertType string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE alerts SET status = 'resolved', resolved_at = NOW()
		 WHERE lampadaire_id = $1 AND type = $2 AND status = 'open'`,
		lampadaireID, alertType)
	return err
}

// SetAlertWorkOrder links an alert to a work order.
func SetAlertWorkOrder(db *sql.DB, alertID, workOrderID int) error {
	_, err := db.Exec(`UPDATE alerts SET work_order_id=$1 WHERE id=$2`, workOrderID, alertID)
	return err
}

// ListAlerts returns alerts with optional filters, including work_order_id.
func ListAlerts(ctx context.Context, db *sql.DB, filters map[string]string) ([]models.Alert, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	argID := 1

	if status := filters["status"]; status != "" {
		where = append(where, "a.status = $"+itoa(argID))
		args = append(args, status)
		argID++
	}
	if severity := filters["severity"]; severity != "" {
		where = append(where, "a.severity = $"+itoa(argID))
		args = append(args, severity)
		argID++
	}
	if lampID := filters["lampadaire_id"]; lampID != "" {
		where = append(where, "a.lampadaire_id = $"+itoa(argID))
		args = append(args, lampID)
		argID++
	}
	_ = argID

	query := `
		SELECT a.id, a.lampadaire_id, a.type, a.severity, a.message, a.status,
		       a.created_at, a.resolved_at, a.acknowledged_at, a.closed_at,
		       COALESCE(a.probable_cause,'') as probable_cause,
		       COALESCE(a.recommended_action,'') as recommended_action,
		       COALESCE(a.source_type,'lampadaire') as source_type,
		       a.work_order_id,
		       COALESCE(l.reference,'') as reference,
		       l.lcu_id, l.zone
		FROM alerts a
		LEFT JOIN lampadaires l ON a.lampadaire_id = l.id
		WHERE ` + joinWhere(where) + `
		ORDER BY a.created_at DESC
		LIMIT 200
	`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	alerts := []models.Alert{}
	for rows.Next() {
		var a models.Alert
		var lid, woID, lcuID sql.NullInt64
		var resolved, acknowledged, closed sql.NullTime
		var zone sql.NullString
		if err := rows.Scan(
			&a.ID, &lid, &a.Type, &a.Severity, &a.Message, &a.Status,
			&a.CreatedAt, &resolved, &acknowledged, &closed,
			&a.ProbableCause, &a.RecommendedAction, &a.SourceType,
			&woID,
			&a.Reference, &lcuID, &zone,
		); err != nil {
			continue
		}
		if lid.Valid {
			v := int(lid.Int64)
			a.LampadaireID = &v
		}
		if woID.Valid {
			v := int(woID.Int64)
			a.WorkOrderID = &v
		}
		if lcuID.Valid {
			v := int(lcuID.Int64)
			a.LCUID = &v
		}
		if zone.Valid {
			a.Zone = zone.String
		}
		if resolved.Valid {
			a.ResolvedAt = &resolved.Time
		}
		if acknowledged.Valid {
			a.AcknowledgedAt = &acknowledged.Time
		}
		if closed.Valid {
			a.ClosedAt = &closed.Time
		}
		alerts = append(alerts, a)
	}

	return alerts, nil
}

func joinWhere(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " AND "
		}
		result += p
	}
	return result
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
