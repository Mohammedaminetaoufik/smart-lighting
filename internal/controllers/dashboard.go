package controllers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/repository"
)

// HandleGetDashboardStats handles GET /api/dashboard/stats.
func HandleGetDashboardStats(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		stats, err := repository.GetDashboardStats(c.Request.Context(), db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors du chargement des statistiques.")
			return
		}
		repository.EnrichDashboardStats(c.Request.Context(), db, stats)
		RespondJSON(c, http.StatusOK, stats)
	}
}

// HandleGetEnergySummary handles GET /api/energy/summary.
func HandleGetEnergySummary(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		summary, err := repository.GetEnergySummary(c.Request.Context(), db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lors du calcul énergétique.")
			return
		}
		RespondJSON(c, http.StatusOK, summary)
	}
}

// HandleGetDailyEnergy handles GET /api/energy/daily?days=30
// Returns daily kWh series + previous period total for trend comparison.
func HandleGetDailyEnergy(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		days := 30
		if d, err := strconv.Atoi(c.Query("days")); err == nil && d > 0 && d <= 365 {
			days = d
		}
		ctx := c.Request.Context()

		type DayEnergy struct {
			Date string  `json:"date"`
			KWh  float64 `json:"kwh"`
		}

		rows, err := db.QueryContext(ctx, `
			SELECT
				DATE(created_at)::text AS day,
				COALESCE(
					NULLIF(SUM(energie), 0),
					SUM(puissance) * 5.0 / 60.0 / 1000.0
				) AS kwh
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
			var d DayEnergy
			if err := rows.Scan(&d.Date, &d.KWh); err == nil {
				byDate[d.Date] = d.KWh
			}
		}

		result := make([]DayEnergy, 0, days)
		for i := days - 1; i >= 0; i-- {
			day := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
			result = append(result, DayEnergy{Date: day, KWh: byDate[day]})
		}

		// Previous period total for trend comparison
		var prevTotal float64
		db.QueryRowContext(ctx, `
			SELECT COALESCE(
				NULLIF(SUM(energie), 0),
				SUM(puissance) * 5.0 / 60.0 / 1000.0
			)
			FROM sensor_measurements
			WHERE created_at >= NOW() - ($1 * INTERVAL '1 day') * 2
			  AND created_at <  NOW() - ($1 * INTERVAL '1 day')
			  AND (energie IS NOT NULL OR puissance IS NOT NULL)
		`, days).Scan(&prevTotal)

		RespondJSON(c, http.StatusOK, gin.H{
			"days":               result,
			"previous_total_kwh": prevTotal,
		})
	}
}

// HandleGetEnergyTopConsumers handles GET /api/energy/top-consumers?days=N&limit=8
func HandleGetEnergyTopConsumers(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		days := 30
		if d, err := strconv.Atoi(c.Query("days")); err == nil && d > 0 && d <= 365 {
			days = d
		}
		limit := 8
		if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 50 {
			limit = l
		}

		type TopConsumer struct {
			ID        int     `json:"id"`
			Reference string  `json:"reference"`
			Zone      string  `json:"zone"`
			Etat      string  `json:"etat"`
			LcuRef    string  `json:"lcu_ref"`
			KWh       float64 `json:"kwh"`
			AvgDimPct float64 `json:"avg_dim_pct"`
		}

		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT
				l.id,
				COALESCE(l.reference, l.id::text) AS reference,
				COALESCE(l.zone, 'Sans zone') AS zone,
				COALESCE(l.etat, 'unknown') AS etat,
				COALESCE(lcu.reference, lcu.name, '') AS lcu_ref,
				COALESCE(
					NULLIF(SUM(sm.energie), 0),
					SUM(sm.puissance) * 5.0 / 60.0 / 1000.0
				) AS kwh,
				COALESCE(AVG(CASE WHEN l.puissance > 0 THEN sm.puissance / l.puissance * 100 ELSE l.intensite END), l.intensite, 0) AS avg_dim_pct
			FROM sensor_measurements sm
			JOIN lampadaires l ON l.id = sm.lampadaire_id
			LEFT JOIN lcus lcu ON lcu.id = l.lcu_id
			WHERE sm.created_at >= NOW() - ($1 * INTERVAL '1 day')
			  AND l.archived_at IS NULL
			  AND (sm.energie IS NOT NULL OR sm.puissance IS NOT NULL)
			GROUP BY l.id, l.reference, l.zone, l.etat, lcu.reference, lcu.name, l.intensite
			ORDER BY kwh DESC
			LIMIT $2
		`, days, limit)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		result := []TopConsumer{}
		for rows.Next() {
			var tc TopConsumer
			if err := rows.Scan(&tc.ID, &tc.Reference, &tc.Zone, &tc.Etat, &tc.LcuRef, &tc.KWh, &tc.AvgDimPct); err == nil {
				result = append(result, tc)
			}
		}
		RespondJSON(c, http.StatusOK, result)
	}
}

// HandleGetEnergyAnomalies handles GET /api/energy/anomalies?days=N
// Returns open alerts linked to lampadaires, ordered by severity.
func HandleGetEnergyAnomalies(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		days := 30
		if d, err := strconv.Atoi(c.Query("days")); err == nil && d > 0 && d <= 365 {
			days = d
		}

		type EnergyAnomaly struct {
			ID        int    `json:"id"`
			Type      string `json:"type"`
			Severity  string `json:"severity"`
			Message   string `json:"message"`
			CreatedAt string `json:"created_at"`
			LampRef   string `json:"lamp_ref"`
			Zone      string `json:"zone"`
			LcuRef    string `json:"lcu_ref"`
		}

		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT
				a.id,
				COALESCE(a.type, 'unknown') AS type,
				COALESCE(a.severity, 'info') AS severity,
				COALESCE(a.message, '') AS message,
				a.created_at::text,
				COALESCE(l.reference, l.id::text) AS lamp_ref,
				COALESCE(l.zone, 'Sans zone') AS zone,
				COALESCE(lcu.reference, lcu.name, '') AS lcu_ref
			FROM alerts a
			JOIN lampadaires l ON l.id = a.lampadaire_id
			LEFT JOIN lcus lcu ON lcu.id = l.lcu_id
			WHERE a.status = 'open'
			  AND a.created_at >= NOW() - ($1 * INTERVAL '1 day')
			ORDER BY
				CASE a.severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END,
				a.created_at DESC
			LIMIT 20
		`, days)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		result := []EnergyAnomaly{}
		for rows.Next() {
			var ea EnergyAnomaly
			if err := rows.Scan(&ea.ID, &ea.Type, &ea.Severity, &ea.Message, &ea.CreatedAt, &ea.LampRef, &ea.Zone, &ea.LcuRef); err == nil {
				result = append(result, ea)
			}
		}
		RespondJSON(c, http.StatusOK, result)
	}
}

// HandleGetEnergyHourly handles GET /api/energy/hourly
// Returns today's consumption broken down by hour (0–23).
func HandleGetEnergyHourly(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		type HourEnergy struct {
			Hour int     `json:"hour"`
			KWh  float64 `json:"kwh"`
		}

		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT
				EXTRACT(HOUR FROM created_at)::int AS hour,
				COALESCE(
					NULLIF(SUM(energie), 0),
					SUM(puissance) * (1.0/12.0) / 1000.0
				) AS kwh
			FROM sensor_measurements
			WHERE created_at >= CURRENT_DATE
			  AND (energie IS NOT NULL OR puissance IS NOT NULL)
			GROUP BY EXTRACT(HOUR FROM created_at)
			ORDER BY hour
		`)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		byHour := map[int]float64{}
		for rows.Next() {
			var he HourEnergy
			if err := rows.Scan(&he.Hour, &he.KWh); err == nil {
				byHour[he.Hour] = he.KWh
			}
		}

		result := make([]HourEnergy, 24)
		for h := 0; h < 24; h++ {
			result[h] = HourEnergy{Hour: h, KWh: byHour[h]}
		}
		RespondJSON(c, http.StatusOK, result)
	}
}

// HandleGetEnergyRecommendations handles GET /api/energy/recommendations
// Generates data-driven recommendations from DB state.
func HandleGetEnergyRecommendations(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		type Recommendation struct {
			Title     string  `json:"title"`
			Detail    string  `json:"detail"`
			SavingKWh float64 `json:"saving_kwh"`
			SavingDH  float64 `json:"saving_dh"`
			Priority  string  `json:"priority"`
			Type      string  `json:"type"`
		}

		const tariff = 1.20
		result := []Recommendation{}

		// 1. Lampadaires without a lighting profile — recommend dimming profile
		profileRows, err := db.QueryContext(ctx, `
			SELECT l.zone, COUNT(*) AS cnt, COALESCE(SUM(l.puissance), 0) AS total_w
			FROM lampadaires l
			WHERE l.lighting_profile_id IS NULL
			  AND l.archived_at IS NULL
			  AND l.etat = 'online'
			GROUP BY l.zone
			ORDER BY total_w DESC
			LIMIT 3
		`)
		if err == nil {
			defer profileRows.Close()
			for profileRows.Next() {
				var zone string
				var cnt int
				var totalW float64
				if profileRows.Scan(&zone, &cnt, &totalW) == nil && cnt > 0 {
					// saving: 45% dimming reduction × 5 night hours
					savKWh := totalW * 0.45 * 5.0 / 1000.0
					result = append(result, Recommendation{
						Title:     "Profil nocturne — " + zone,
						Detail:    fmt.Sprintf("Réduire à 45%% entre 00h–05h pour %d lampadaires sans profil configuré", cnt),
						SavingKWh: savKWh,
						SavingDH:  savKWh * tariff,
						Priority:  "high",
						Type:      "profile",
					})
				}
			}
		}

		// 2. High fixed intensity (> 80%) with no recent dimming command
		var highCnt int
		var highW float64
		db.QueryRowContext(ctx, `
			SELECT COUNT(*), COALESCE(SUM(puissance), 0)
			FROM lampadaires l
			WHERE l.intensite > 80
			  AND l.archived_at IS NULL
			  AND l.etat = 'online'
			  AND NOT EXISTS (
				SELECT 1 FROM dimming_commands dc
				WHERE dc.lampadaire_id = l.id
				  AND dc.created_at >= NOW() - INTERVAL '7 days'
			  )
		`).Scan(&highCnt, &highW)
		if highCnt > 0 {
			savKWh := highW * 0.30 * 8.0 / 1000.0
			result = append(result, Recommendation{
				Title:     "Intensité trop élevée — " + strconv.Itoa(highCnt) + " lampadaires",
				Detail:    fmt.Sprintf("%d lampadaires à plus de 80%% sans commande de dimming depuis 7 jours", highCnt),
				SavingKWh: savKWh,
				SavingDH:  savKWh * tariff,
				Priority:  "medium",
				Type:      "dimming",
			})
		}

		// 3. Offline lamps with open alerts — urgent intervention
		var offlineCnt int
		var offlineW float64
		db.QueryRowContext(ctx, `
			SELECT COUNT(DISTINCT l.id), COALESCE(SUM(l.puissance), 0)
			FROM lampadaires l
			JOIN alerts a ON a.lampadaire_id = l.id
			WHERE l.etat = 'offline'
			  AND l.archived_at IS NULL
			  AND a.status = 'open'
		`).Scan(&offlineCnt, &offlineW)
		if offlineCnt > 0 {
			result = append(result, Recommendation{
				Title:     "Intervention urgente — " + strconv.Itoa(offlineCnt) + " lampadaires hors ligne",
				Detail:    fmt.Sprintf("%d lampadaires offline avec alertes ouvertes — vérification terrain requise", offlineCnt),
				SavingKWh: 0,
				SavingDH:  0,
				Priority:  "high",
				Type:      "intervention",
			})
		}

		RespondJSON(c, http.StatusOK, result)
	}
}

// HandleGetNetworkHealth handles GET /api/dashboard/network-health.
func HandleGetNetworkHealth(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		type NetworkHealth struct {
			TotalBasestations      int     `json:"total_basestations"`
			BasestationsOnline     int     `json:"basestations_online"`
			BasestationsOffline    int     `json:"basestations_offline"`
			TotalControllers       int     `json:"total_controllers"`
			ControllersOK          int     `json:"controllers_ok"`
			ControllersLost        int     `json:"controllers_lost"`
			AvgSignalQuality       float64 `json:"avg_signal_quality"`
			NetworkAvailabilityPct float64 `json:"network_availability_pct"`
		}
		var h NetworkHealth
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM basestations`).Scan(&h.TotalBasestations)
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM basestations WHERE status='online'`).Scan(&h.BasestationsOnline)
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM basestations WHERE status='offline'`).Scan(&h.BasestationsOffline)
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM controllers`).Scan(&h.TotalControllers)
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM controllers WHERE communication_status='ok'`).Scan(&h.ControllersOK)
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM controllers WHERE communication_status='lost'`).Scan(&h.ControllersLost)
		db.QueryRowContext(ctx, `SELECT COALESCE(AVG(signal_quality),0) FROM controllers`).Scan(&h.AvgSignalQuality)
		if h.TotalBasestations > 0 {
			h.NetworkAvailabilityPct = float64(h.BasestationsOnline) / float64(h.TotalBasestations) * 100
		}
		RespondJSON(c, http.StatusOK, h)
	}
}

// HandleGetCommissioningProgress handles GET /api/dashboard/commissioning-progress.
func HandleGetCommissioningProgress(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		type StepCount struct {
			Status string `json:"status"`
			Count  int    `json:"count"`
		}
		rows, err := db.QueryContext(ctx, `
			SELECT commissioning_status, COUNT(*) as cnt
			FROM lampadaires WHERE archived_at IS NULL
			GROUP BY commissioning_status ORDER BY commissioning_status`)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()
		var steps []StepCount
		total := 0
		commissioned := 0
		for rows.Next() {
			var s StepCount
			if err := rows.Scan(&s.Status, &s.Count); err == nil {
				steps = append(steps, s)
				total += s.Count
				if s.Status == "commissioned" {
					commissioned = s.Count
				}
			}
		}
		rate := 0.0
		if total > 0 {
			rate = float64(commissioned) / float64(total) * 100
		}
		RespondJSON(c, http.StatusOK, gin.H{
			"steps":              steps,
			"total":              total,
			"commissioned":       commissioned,
			"commissioning_rate": rate,
		})
	}
}
