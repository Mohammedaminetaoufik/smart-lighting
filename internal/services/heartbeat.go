package services

import (
	"context"
	"database/sql"
	"log"
)

// Heartbeat records a successful (or failed) run for a named background job.
// Failures are logged but never returned — observability is best-effort.
func Heartbeat(ctx context.Context, db *sql.DB, name, status, message string) {
	_, err := db.ExecContext(ctx, `
		INSERT INTO job_heartbeats (name, last_run_at, status, message, updated_at)
		VALUES ($1, NOW(), $2, NULLIF($3, ''), NOW())
		ON CONFLICT (name) DO UPDATE
		SET last_run_at = NOW(), status = EXCLUDED.status,
		    message = EXCLUDED.message, updated_at = NOW()`,
		name, status, message)
	if err != nil {
		log.Printf("heartbeat error (job=%s): %v", name, err)
	}
}
