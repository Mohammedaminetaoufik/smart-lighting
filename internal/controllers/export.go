package controllers

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// writeCSVHeader sets headers and returns a CSV writer.
func writeCSVHeader(c *gin.Context, filename string, columns []string) *csv.Writer {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s_%s.csv"`,
		filename, time.Now().Format("20060102_150405")))
	// BOM so Excel opens UTF-8 correctly
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(c.Writer)
	_ = w.Write(columns)
	return w
}

func nullableString(s sql.NullString) string {
	if s.Valid {
		return s.String
	}
	return ""
}

func nullableInt(n sql.NullInt64) string {
	if n.Valid {
		return fmt.Sprintf("%d", n.Int64)
	}
	return ""
}

func nullableFloat(f sql.NullFloat64) string {
	if f.Valid {
		return fmt.Sprintf("%.6f", f.Float64)
	}
	return ""
}

func nullableTime(t sql.NullTime) string {
	if t.Valid {
		return t.Time.Format(time.RFC3339)
	}
	return ""
}

// HandleExportLampadaires handles GET /api/export/lampadaires.
func HandleExportLampadaires(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		filters := []string{"archived_at IS NULL"}
		args := []any{}
		if etat := c.Query("etat"); etat != "" {
			filters = append(filters, fmt.Sprintf("etat=$%d", len(args)+1))
			args = append(args, etat)
		}
		if zone := c.Query("zone"); zone != "" {
			filters = append(filters, fmt.Sprintf("zone=$%d", len(args)+1))
			args = append(args, zone)
		}
		query := `SELECT id, reference, latitude, longitude, zone, etat, intensite,
			puissance, commissioning_status, location_status, last_seen_at, created_at
			FROM lampadaires WHERE ` + strings.Join(filters, " AND ") + " ORDER BY id"

		rows, err := db.QueryContext(c.Request.Context(), query, args...)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		w := writeCSVHeader(c, "lampadaires", []string{
			"id", "reference", "latitude", "longitude", "zone", "etat", "intensite",
			"puissance", "commissioning_status", "location_status", "last_seen_at", "created_at",
		})

		for rows.Next() {
			var id, intensite int
			var ref, etat, commStatus, locStatus string
			var lat, lng sql.NullFloat64
			var zone sql.NullString
			var puissance sql.NullInt64
			var lastSeen sql.NullTime
			var createdAt time.Time
			if err := rows.Scan(&id, &ref, &lat, &lng, &zone, &etat, &intensite,
				&puissance, &commStatus, &locStatus, &lastSeen, &createdAt); err != nil {
				continue
			}
			_ = w.Write([]string{
				fmt.Sprintf("%d", id), ref, nullableFloat(lat), nullableFloat(lng),
				nullableString(zone), etat, fmt.Sprintf("%d", intensite),
				nullableInt(puissance), commStatus, locStatus,
				nullableTime(lastSeen), createdAt.Format(time.RFC3339),
			})
		}
		w.Flush()
	}
}

// HandleExportAlerts handles GET /api/export/alerts.
func HandleExportAlerts(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		filters := []string{"1=1"}
		args := []any{}
		if sev := c.Query("severity"); sev != "" {
			filters = append(filters, fmt.Sprintf("a.severity=$%d", len(args)+1))
			args = append(args, sev)
		}
		if st := c.Query("status"); st != "" {
			filters = append(filters, fmt.Sprintf("a.status=$%d", len(args)+1))
			args = append(args, st)
		}
		if from := c.Query("from"); from != "" {
			filters = append(filters, fmt.Sprintf("a.created_at >= $%d", len(args)+1))
			args = append(args, from)
		}
		if to := c.Query("to"); to != "" {
			filters = append(filters, fmt.Sprintf("a.created_at <= $%d", len(args)+1))
			args = append(args, to)
		}
		query := `SELECT a.id, COALESCE(l.reference,''), a.type, a.severity, a.status,
			a.message, a.created_at, a.acknowledged_at, a.resolved_at, a.closed_at
			FROM alerts a
			LEFT JOIN lampadaires l ON l.id = a.lampadaire_id
			WHERE ` + strings.Join(filters, " AND ") + " ORDER BY a.created_at DESC"

		rows, err := db.QueryContext(c.Request.Context(), query, args...)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		w := writeCSVHeader(c, "alerts", []string{
			"id", "lampadaire_ref", "type", "severity", "status", "message",
			"created_at", "acknowledged_at", "resolved_at", "closed_at",
		})
		for rows.Next() {
			var id int
			var ref, alertType, sev, status, msg string
			var createdAt time.Time
			var ackAt, resAt, closedAt sql.NullTime
			if err := rows.Scan(&id, &ref, &alertType, &sev, &status, &msg,
				&createdAt, &ackAt, &resAt, &closedAt); err != nil {
				continue
			}
			_ = w.Write([]string{
				fmt.Sprintf("%d", id), ref, alertType, sev, status, msg,
				createdAt.Format(time.RFC3339),
				nullableTime(ackAt), nullableTime(resAt), nullableTime(closedAt),
			})
		}
		w.Flush()
	}
}

// HandleExportEnergy handles GET /api/export/energy?days=N
// Returns a CSV with daily kWh, estimated, savings, cost and CO2.
func HandleExportEnergy(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		days := 30
		if d, err := strconv.Atoi(c.Query("days")); err == nil && d > 0 && d <= 365 {
			days = d
		}

		const tariff = 1.20
		const co2Coef = 0.52

		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT
				DATE(created_at)::text AS day,
				COALESCE(
					NULLIF(SUM(energie), 0),
					SUM(puissance) * 5.0 / 60.0 / 1000.0
				) AS kwh_real
			FROM sensor_measurements
			WHERE created_at >= NOW() - ($1 * INTERVAL '1 day')
			  AND (energie IS NOT NULL OR puissance IS NOT NULL)
			GROUP BY DATE(created_at)
			ORDER BY day
		`, days)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		byDate := map[string]float64{}
		for rows.Next() {
			var day string
			var kwh float64
			if rows.Scan(&day, &kwh) == nil {
				byDate[day] = kwh
			}
		}

		w := writeCSVHeader(c, "energie", []string{
			"Date", "kWh réel", "kWh estimé (sans dimming)", "Économies kWh", "Coût DH", "CO2 évité kg",
		})
		for i := days - 1; i >= 0; i-- {
			day := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
			real := byDate[day]
			estimated := real * 1.38
			savings := estimated - real
			_ = w.Write([]string{
				day,
				fmt.Sprintf("%.3f", real),
				fmt.Sprintf("%.3f", estimated),
				fmt.Sprintf("%.3f", savings),
				fmt.Sprintf("%.2f", real*tariff),
				fmt.Sprintf("%.3f", savings*co2Coef),
			})
		}
		w.Flush()
	}
}

// HandleExportWorkOrders handles GET /api/export/workorders.
func HandleExportWorkOrders(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		filters := []string{"1=1"}
		args := []any{}
		if st := c.Query("status"); st != "" {
			filters = append(filters, fmt.Sprintf("w.status=$%d", len(args)+1))
			args = append(args, st)
		}
		if prio := c.Query("priority"); prio != "" {
			filters = append(filters, fmt.Sprintf("w.priority=$%d", len(args)+1))
			args = append(args, prio)
		}
		query := `SELECT w.id, w.title, w.priority, w.status,
			COALESCE(l.reference,''), COALESCE(u.full_name,''),
			w.created_at, w.due_date, w.closed_at, COALESCE(w.resolution_note, '')
			FROM work_orders w
			LEFT JOIN lampadaires l ON l.id = w.lampadaire_id
			LEFT JOIN users u ON u.id = w.assigned_to
			WHERE ` + strings.Join(filters, " AND ") + " ORDER BY w.id DESC"

		rows, err := db.QueryContext(c.Request.Context(), query, args...)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		w := writeCSVHeader(c, "workorders", []string{
			"id", "title", "priority", "status", "lampadaire_ref", "assigned_to",
			"created_at", "due_date", "closed_at", "resolution_note",
		})
		for rows.Next() {
			var id int
			var title, priority, status, ref, assignee, note string
			var createdAt time.Time
			var dueDate, closedAt sql.NullTime
			if err := rows.Scan(&id, &title, &priority, &status, &ref, &assignee,
				&createdAt, &dueDate, &closedAt, &note); err != nil {
				continue
			}
			_ = w.Write([]string{
				fmt.Sprintf("%d", id), title, priority, status, ref, assignee,
				createdAt.Format(time.RFC3339),
				nullableTime(dueDate), nullableTime(closedAt), note,
			})
		}
		w.Flush()
	}
}
