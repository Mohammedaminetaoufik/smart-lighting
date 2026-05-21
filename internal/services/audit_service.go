package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuditLogInput holds all fields for a new audit_logs entry.
type AuditLogInput struct {
	UserID          *int
	UserName        string
	UserRole        string
	Action          string
	EntityType      string
	EntityID        *int
	EntityReference string
	Description     string
	OldValues       any
	NewValues       any
	Status          string // "success" | "error" — defaults to "success"
	IPAddress       string
	UserAgent       string
}

// AuditCtx holds request-scoped identity extracted from a gin.Context.
type AuditCtx struct {
	UserID    *int
	UserName  string
	UserRole  string
	IPAddress string
	UserAgent string
}

// GetAuditContext extracts user identity and request metadata from a Gin context.
// Works with or without auth middleware — fields are simply empty when unauthenticated.
func GetAuditContext(c *gin.Context) AuditCtx {
	ac := AuditCtx{
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}
	if id, ok := c.Get("user_id"); ok {
		if v, ok2 := id.(int); ok2 {
			ac.UserID = &v
		}
	}
	if name, ok := c.Get("user_name"); ok {
		if v, ok2 := name.(string); ok2 {
			ac.UserName = v
		}
	}
	if role, ok := c.Get("user_role"); ok {
		if v, ok2 := role.(string); ok2 {
			ac.UserRole = v
		}
	}
	return ac
}

// LogAudit writes one row to audit_logs. Failures are logged but non-fatal.
func LogAudit(ctx context.Context, db *sql.DB, input AuditLogInput) {
	status := input.Status
	if status == "" {
		status = "success"
	}

	oldJSON := marshalAuditValues(input.OldValues)
	newJSON := marshalAuditValues(input.NewValues)

	_, err := db.ExecContext(ctx, `
		INSERT INTO audit_logs
			(user_id, user_name, user_role, action, entity_type, entity_id,
			 entity_reference, description, old_values, new_values, status,
			 ip_address, user_agent)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		auditNullInt(input.UserID), auditNullStr(input.UserName), auditNullStr(input.UserRole),
		input.Action, input.EntityType, auditNullInt(input.EntityID),
		auditNullStr(input.EntityReference), input.Description,
		oldJSON, newJSON,
		status,
		auditNullStr(input.IPAddress), auditNullStr(input.UserAgent),
	)
	if err != nil {
		log.Printf("audit_log error: %v (action=%s entity=%s)", err, input.Action, input.EntityType)
	}
}

// marshalAuditValues serialises a value to JSON bytes for JSONB columns.
// Returns nil for nil input; strips keys with nil/empty-string values.
func marshalAuditValues(v any) []byte {
	if v == nil {
		return nil
	}
	sanitized := sanitizeValues(v)
	b, err := json.Marshal(sanitized)
	if err != nil {
		return nil
	}
	if string(b) == "null" || string(b) == "{}" {
		return nil
	}
	return b
}

// sanitizeValues recursively removes nil values and empty strings from maps.
func sanitizeValues(v any) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, vv := range val {
			if vv == nil {
				continue
			}
			if s, ok := vv.(string); ok && strings.TrimSpace(s) == "" {
				continue
			}
			out[k] = sanitizeValues(vv)
		}
		return out
	case []any:
		out := make([]any, 0, len(val))
		for _, item := range val {
			out = append(out, sanitizeValues(item))
		}
		return out
	default:
		return v
	}
}

func auditNullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func auditNullInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
