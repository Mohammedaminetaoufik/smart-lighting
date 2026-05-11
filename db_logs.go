package main

import (
	"context"
	"database/sql"
)

func logAccess(ctx context.Context, db *sql.DB, userID int, action string) error {
	_, err := db.ExecContext(ctx, "INSERT INTO access_logs (user_id, action) VALUES (, )", userID, action)
	return err
}

func listAccessLogs(ctx context.Context, db *sql.DB) ([]AccessLog, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, user_id, action, created_at FROM access_logs ORDER BY created_at DESC LIMIT 50")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []AccessLog
	for rows.Next() {
		var l AccessLog
		if err := rows.Scan(&l.ID, &l.UserID, &l.Action, &l.CreatedAt); err != nil {
			return nil, err
		}
		res = append(res, l)
	}
	return res, nil
}
