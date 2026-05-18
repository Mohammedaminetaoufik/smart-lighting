package controllers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/models"
)

// HandleGetAuditLogs handles GET /api/audit-logs with filters & pagination.
// Query params: user_id, action, target_type, from, to, limit (default 50), offset (default 0)
func HandleGetAuditLogs(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		filters := []string{"1=1"}
		args := []any{}
		argIdx := 1

		if v := c.Query("user_id"); v != "" {
			if id, err := strconv.Atoi(v); err == nil {
				filters = append(filters, "al.user_id = $"+strconv.Itoa(argIdx))
				args = append(args, id)
				argIdx++
			}
		}
		if v := c.Query("action"); v != "" {
			filters = append(filters, "al.action ILIKE $"+strconv.Itoa(argIdx))
			args = append(args, "%"+v+"%")
			argIdx++
		}
		if v := c.Query("target_type"); v != "" {
			filters = append(filters, "al.target_type = $"+strconv.Itoa(argIdx))
			args = append(args, v)
			argIdx++
		}
		if v := c.Query("from"); v != "" {
			filters = append(filters, "al.created_at >= $"+strconv.Itoa(argIdx))
			args = append(args, v)
			argIdx++
		}
		if v := c.Query("to"); v != "" {
			filters = append(filters, "al.created_at <= $"+strconv.Itoa(argIdx))
			args = append(args, v)
			argIdx++
		}

		limit := 50
		if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 && v <= 500 {
			limit = v
		}
		offset := 0
		if v, err := strconv.Atoi(c.Query("offset")); err == nil && v >= 0 {
			offset = v
		}

		whereClause := ""
		for i, f := range filters {
			if i == 0 {
				whereClause = "WHERE " + f
			} else {
				whereClause += " AND " + f
			}
		}

		// Count total for pagination
		countQuery := "SELECT COUNT(*) FROM access_logs al " + whereClause
		var total int
		if err := db.QueryRowContext(c.Request.Context(), countQuery, args...).Scan(&total); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur comptage: "+err.Error())
			return
		}

		query := `SELECT al.id, al.user_id, COALESCE(u.full_name, ''), al.action,
			COALESCE(al.target_type, ''), al.target_id,
			COALESCE(al.ip_address, ''), COALESCE(al.user_agent, ''),
			al.metadata, al.created_at
			FROM access_logs al
			LEFT JOIN users u ON u.id = al.user_id
			` + whereClause + ` ORDER BY al.created_at DESC LIMIT $` + strconv.Itoa(argIdx) +
			` OFFSET $` + strconv.Itoa(argIdx+1)

		argsWithLimit := append(args, limit, offset)
		rows, err := db.QueryContext(c.Request.Context(), query, argsWithLimit...)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur recherche: "+err.Error())
			return
		}
		defer rows.Close()

		logs := []models.AccessLog{}
		for rows.Next() {
			var l models.AccessLog
			var userID sql.NullInt64
			var targetID sql.NullInt64
			var metaRaw sql.NullString
			var createdAt time.Time
			if err := rows.Scan(&l.ID, &userID, &l.UserName, &l.Action,
				&l.TargetType, &targetID,
				&l.IPAddress, &l.UserAgent,
				&metaRaw, &createdAt); err != nil {
				continue
			}
			l.CreatedAt = createdAt.Format(time.RFC3339)
			if userID.Valid {
				uid := int(userID.Int64)
				l.UserID = &uid
			}
			if targetID.Valid {
				tid := int(targetID.Int64)
				l.TargetID = &tid
			}
			if metaRaw.Valid && metaRaw.String != "" {
				var m map[string]interface{}
				if json.Unmarshal([]byte(metaRaw.String), &m) == nil {
					l.Metadata = m
				}
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
