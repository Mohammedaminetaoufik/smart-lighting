package repository

import (
	"context"
	"database/sql"

	"map-interactif/internal/models"
)

// LogAccess inserts an access log entry.
func LogAccess(ctx context.Context, db *sql.DB, userID int, action string) error {
	_, err := db.ExecContext(ctx, "INSERT INTO access_logs (user_id, action) VALUES ($1, $2)", userID, action)
	return err
}

// ListAccessLogs returns the 50 most recent access logs.
func ListAccessLogs(ctx context.Context, db *sql.DB) ([]models.AccessLog, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, user_id, action, created_at FROM access_logs ORDER BY created_at DESC LIMIT 50")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []models.AccessLog
	for rows.Next() {
		var l models.AccessLog
		if err := rows.Scan(&l.ID, &l.UserID, &l.Action, &l.CreatedAt); err != nil {
			return nil, err
		}
		res = append(res, l)
	}
	return res, nil
}
