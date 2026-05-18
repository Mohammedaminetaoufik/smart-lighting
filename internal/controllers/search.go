package controllers

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type searchHit struct {
	Type     string `json:"type"`
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	URL      string `json:"url"`
}

// HandleGlobalSearch handles GET /api/search?q=...
// Returns up to 5 hits per domain (lampadaires, lcus, alerts, work_orders, users).
func HandleGlobalSearch(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := strings.TrimSpace(c.Query("q"))
		if len(q) < 2 {
			RespondJSON(c, http.StatusOK, gin.H{"results": []searchHit{}})
			return
		}
		pattern := "%" + q + "%"
		results := []searchHit{}

		// Lampadaires
		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT id, reference, COALESCE(zone, ''), etat
			FROM lampadaires
			WHERE archived_at IS NULL AND (reference ILIKE $1 OR zone ILIKE $1)
			ORDER BY id LIMIT 5`, pattern)
		if err == nil {
			for rows.Next() {
				var h searchHit
				var zone, etat string
				if err := rows.Scan(&h.ID, &h.Title, &zone, &etat); err == nil {
					h.Type = "lampadaire"
					h.Subtitle = strings.TrimSpace(zone + " · " + etat)
					h.URL = "/lampadaires"
					results = append(results, h)
				}
			}
			rows.Close()
		}

		// LCUs
		rows2, err := db.QueryContext(c.Request.Context(), `
			SELECT id, reference, COALESCE(name, ''), COALESCE(ip_address, '')
			FROM lcus WHERE reference ILIKE $1 OR name ILIKE $1
			ORDER BY id LIMIT 5`, pattern)
		if err == nil {
			for rows2.Next() {
				var h searchHit
				var name, ip string
				if err := rows2.Scan(&h.ID, &h.Title, &name, &ip); err == nil {
					h.Type = "lcu"
					h.Subtitle = strings.TrimSpace(name + " · " + ip)
					h.URL = "/lcus"
					results = append(results, h)
				}
			}
			rows2.Close()
		}

		// Alerts (open + recent)
		rows3, err := db.QueryContext(c.Request.Context(), `
			SELECT a.id, a.message, a.severity, COALESCE(l.reference, '')
			FROM alerts a
			LEFT JOIN lampadaires l ON l.id = a.lampadaire_id
			WHERE (a.message ILIKE $1 OR a.type ILIKE $1)
			AND a.status IN ('open', 'acknowledged', 'in_progress')
			ORDER BY a.created_at DESC LIMIT 5`, pattern)
		if err == nil {
			for rows3.Next() {
				var h searchHit
				var sev, lampRef string
				if err := rows3.Scan(&h.ID, &h.Title, &sev, &lampRef); err == nil {
					h.Type = "alert"
					h.Subtitle = strings.TrimSpace(sev + " · " + lampRef)
					h.URL = "/alerts"
					results = append(results, h)
				}
			}
			rows3.Close()
		}

		// Work orders
		rows4, err := db.QueryContext(c.Request.Context(), `
			SELECT id, title, priority, status
			FROM work_orders WHERE title ILIKE $1
			ORDER BY id DESC LIMIT 5`, pattern)
		if err == nil {
			for rows4.Next() {
				var h searchHit
				var priority, status string
				if err := rows4.Scan(&h.ID, &h.Title, &priority, &status); err == nil {
					h.Type = "workorder"
					h.Subtitle = priority + " · " + status
					h.URL = "/workorders"
					results = append(results, h)
				}
			}
			rows4.Close()
		}

		// Users
		rows5, err := db.QueryContext(c.Request.Context(), `
			SELECT id, full_name, email, role
			FROM users WHERE (full_name ILIKE $1 OR email ILIKE $1)
			AND COALESCE(status, 'active') != 'deleted'
			ORDER BY id LIMIT 5`, pattern)
		if err == nil {
			for rows5.Next() {
				var h searchHit
				var email, role string
				if err := rows5.Scan(&h.ID, &h.Title, &email, &role); err == nil {
					h.Type = "user"
					h.Subtitle = email + " · " + role
					h.URL = "/users"
					results = append(results, h)
				}
			}
			rows5.Close()
		}

		RespondJSON(c, http.StatusOK, gin.H{"results": results, "count": len(results)})
	}
}
