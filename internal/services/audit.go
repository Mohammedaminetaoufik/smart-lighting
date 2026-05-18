package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
)

// AuditEvent represents a single audit log entry.
type AuditEvent struct {
	UserID     *int
	Action     string
	TargetType string
	TargetID   *int
	IPAddress  string
	UserAgent  string
	Metadata   map[string]interface{}
}

// LogAction writes an audit event to access_logs. Failures are logged but
// non-fatal — audit logging should never block business operations.
func LogAction(ctx context.Context, db *sql.DB, evt AuditEvent) {
	var metaJSON []byte
	if len(evt.Metadata) > 0 {
		b, err := json.Marshal(evt.Metadata)
		if err == nil {
			metaJSON = b
		}
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO access_logs (user_id, action, target_type, target_id, ip_address, user_agent, metadata)
		VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, ''), NULLIF($6, ''), $7)`,
		evt.UserID, evt.Action, evt.TargetType, evt.TargetID,
		evt.IPAddress, evt.UserAgent, metaJSON)
	if err != nil {
		log.Printf("audit log error: %v (action=%s)", err, evt.Action)
	}
}
