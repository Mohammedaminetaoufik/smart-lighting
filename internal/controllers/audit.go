package controllers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/models"
)

// HandleGetAuditLogs handles GET /api/audit-logs
// Query params: user_id, action, entity_type, status, from, to, limit (default 50, max 500), offset
func HandleGetAuditLogs(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		filters := []string{}
		args := []any{}
		idx := 1

		add := func(clause string, val any) {
			filters = append(filters, clause)
			args = append(args, val)
			idx++
		}

		if v := c.Query("user_id"); v != "" {
			if id, err := strconv.Atoi(v); err == nil {
				add("user_id = $"+strconv.Itoa(idx), id)
			}
		}
		if v := c.Query("action"); v != "" {
			add("action ILIKE $"+strconv.Itoa(idx), "%"+v+"%")
		}
		if v := c.Query("entity_type"); v != "" {
			add("entity_type = $"+strconv.Itoa(idx), v)
		}
		if v := c.Query("status"); v != "" {
			add("status = $"+strconv.Itoa(idx), v)
		}
		if v := c.Query("from"); v != "" {
			add("created_at >= $"+strconv.Itoa(idx), v)
		}
		if v := c.Query("to"); v != "" {
			add("created_at <= $"+strconv.Itoa(idx), v)
		}

		where := buildWhere(filters)

		limit := 50
		if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 && v <= 500 {
			limit = v
		}
		offset := 0
		if v, err := strconv.Atoi(c.Query("offset")); err == nil && v >= 0 {
			offset = v
		}

		var total int
		if err := db.QueryRowContext(c.Request.Context(),
			"SELECT COUNT(*) FROM audit_logs "+where, args...).Scan(&total); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur comptage: "+err.Error())
			return
		}

		argsPage := append(args, limit, offset)
		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT id, user_id, COALESCE(user_name,''), COALESCE(user_role,''),
			       action, entity_type, entity_id, COALESCE(entity_reference,''),
			       description, old_values, new_values, status,
			       COALESCE(ip_address,''), COALESCE(user_agent,''), created_at
			FROM audit_logs `+where+`
			ORDER BY created_at DESC
			LIMIT $`+strconv.Itoa(idx)+` OFFSET $`+strconv.Itoa(idx+1),
			argsPage...)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur requête: "+err.Error())
			return
		}
		defer rows.Close()

		logs := []models.AuditLog{}
		for rows.Next() {
			l, err := scanAuditLog(rows)
			if err != nil {
				continue
			}
			logs = append(logs, l)
		}

		RespondJSON(c, http.StatusOK, gin.H{
			"logs":   logs,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		})
	}
}

// HandleGetAuditLog handles GET /api/audit-logs/:id
func HandleGetAuditLog(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ID invalide")
			return
		}
		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT id, user_id, COALESCE(user_name,''), COALESCE(user_role,''),
			       action, entity_type, entity_id, COALESCE(entity_reference,''),
			       description, old_values, new_values, status,
			       COALESCE(ip_address,''), COALESCE(user_agent,''), created_at
			FROM audit_logs WHERE id = $1`, id)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		if !rows.Next() {
			RespondError(c, http.StatusNotFound, "Journal introuvable")
			return
		}
		l, err := scanAuditLog(rows)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		RespondJSON(c, http.StatusOK, l)
	}
}

// HandleGetAuditSummary handles GET /api/audit-logs/summary
// Returns counts by action and entity_type for the last 30 days.
func HandleGetAuditSummary(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT action, COUNT(*) as count
			FROM audit_logs
			WHERE created_at >= NOW() - INTERVAL '30 days'
			GROUP BY action
			ORDER BY count DESC
			LIMIT 20`)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		byAction := []gin.H{}
		for rows.Next() {
			var action string
			var count int
			if rows.Scan(&action, &count) == nil {
				byAction = append(byAction, gin.H{"action": action, "count": count})
			}
		}

		rows2, err := db.QueryContext(c.Request.Context(), `
			SELECT entity_type, COUNT(*) as count
			FROM audit_logs
			WHERE created_at >= NOW() - INTERVAL '30 days'
			GROUP BY entity_type
			ORDER BY count DESC`)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows2.Close()
		byEntity := []gin.H{}
		for rows2.Next() {
			var et string
			var count int
			if rows2.Scan(&et, &count) == nil {
				byEntity = append(byEntity, gin.H{"entity_type": et, "count": count})
			}
		}

		var totalToday int
		db.QueryRowContext(c.Request.Context(),
			"SELECT COUNT(*) FROM audit_logs WHERE created_at >= CURRENT_DATE").Scan(&totalToday)

		var totalWeek int
		db.QueryRowContext(c.Request.Context(),
			"SELECT COUNT(*) FROM audit_logs WHERE created_at >= NOW() - INTERVAL '7 days'").Scan(&totalWeek)

		RespondJSON(c, http.StatusOK, gin.H{
			"by_action":   byAction,
			"by_entity":   byEntity,
			"total_today": totalToday,
			"total_week":  totalWeek,
		})
	}
}

// scanAuditLog scans a row from audit_logs into a models.AuditLog.
func scanAuditLog(rows *sql.Rows) (models.AuditLog, error) {
	var l models.AuditLog
	var userID, entityID sql.NullInt64
	var oldRaw, newRaw sql.NullString
	var createdAt time.Time
	err := rows.Scan(
		&l.ID, &userID, &l.UserName, &l.UserRole,
		&l.Action, &l.EntityType, &entityID, &l.EntityReference,
		&l.Description, &oldRaw, &newRaw, &l.Status,
		&l.IPAddress, &l.UserAgent, &createdAt,
	)
	if err != nil {
		return l, err
	}
	l.CreatedAt = createdAt.Format(time.RFC3339)
	if userID.Valid {
		v := int(userID.Int64)
		l.UserID = &v
	}
	if entityID.Valid {
		v := int(entityID.Int64)
		l.EntityID = &v
	}
	if oldRaw.Valid && oldRaw.String != "" {
		var m map[string]interface{}
		if json.Unmarshal([]byte(oldRaw.String), &m) == nil {
			l.OldValues = m
		}
	}
	if newRaw.Valid && newRaw.String != "" {
		var m map[string]interface{}
		if json.Unmarshal([]byte(newRaw.String), &m) == nil {
			l.NewValues = m
		}
	}
	return l, nil
}

func buildWhere(filters []string) string {
	if len(filters) == 0 {
		return ""
	}
	return "WHERE " + strings.Join(filters, " AND ")
}
