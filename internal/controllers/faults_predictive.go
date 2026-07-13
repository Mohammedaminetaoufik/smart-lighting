package controllers

import (
	"context"
	"database/sql"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

const faultModelVersion = "Règles v1 (seuils dataset)"

// ── Deterministic scoring helpers (documented, no randomness) ────────────────

// faultSeverityBase gives the base risk contribution of a fault type.
func faultSeverityBase(code string) float64 {
	switch code {
	case "overcurrent", "leakage":
		return 72
	case "overvoltage", "overtemp":
		return 62
	case "underpower":
		return 52
	default:
		return 40
	}
}

// computeRiskScore blends fault severity, relative fault frequency and recency
// into a 0..100 score.
func computeRiskScore(faultStatus string, faultCount, maxFaultCount int, lastAt *time.Time) float64 {
	score := faultSeverityBase(faultStatus)
	if maxFaultCount > 0 {
		score += (float64(faultCount) / float64(maxFaultCount)) * 22
	}
	if lastAt != nil {
		d := time.Since(*lastAt)
		switch {
		case d < 7*24*time.Hour:
			score += 8
		case d < 30*24*time.Hour:
			score += 4
		}
	}
	return math.Min(100, math.Round(score))
}

func riskLevel(score float64, hasFault bool) string {
	if !hasFault {
		return "unknown"
	}
	switch {
	case score >= 85:
		return "critical"
	case score >= 70:
		return "high"
	case score >= 50:
		return "moderate"
	default:
		return "low"
	}
}

func etaFromScore(score float64) (int, string) {
	switch {
	case score >= 85:
		return 168, "Moins de 7 jours"
	case score >= 70:
		return 720, "7 à 30 jours"
	case score >= 50:
		return 2160, "30 à 90 jours"
	default:
		return 0, "Plus de 90 jours"
	}
}

func telemetryFreshness(last *time.Time) string {
	if last == nil {
		return "unavailable"
	}
	d := time.Since(*last)
	switch {
	case d < 15*time.Minute:
		return "fresh"
	case d < 2*time.Hour:
		return "delayed"
	case d < 24*time.Hour:
		return "stale"
	default:
		return "obsolete"
	}
}

// adjustedConfidence caps confidence at 99% and reduces it when the underlying
// telemetry is stale — reliability depends on data freshness.
func adjustedConfidence(avgConf float64, freshness string) float64 {
	conf := avgConf * 100
	switch freshness {
	case "obsolete", "unavailable":
		conf *= 0.7
	case "stale":
		conf *= 0.85
	}
	if conf > 99 {
		conf = 99
	}
	if conf < 40 && conf > 0 {
		conf = 40
	}
	return math.Round(conf)
}

// prediction is the enriched per-lamp predictive record.
type prediction struct {
	ID                 int        `json:"id"`
	Reference          string     `json:"reference"`
	Zone               string     `json:"zone"`
	LCUReference       string     `json:"lcu_reference"`
	Online             bool       `json:"online"`
	FaultStatus        string     `json:"fault_status"`
	PredictedLabel     string     `json:"predicted_label"`
	RiskScore          float64    `json:"risk_score"`
	RiskLevel          string     `json:"risk_level"`
	Confidence         float64    `json:"confidence"`
	ETAHours           int        `json:"eta_hours"`
	ETALabel           string     `json:"eta_label"`
	FaultCount         int        `json:"fault_count"`
	LastTelemetryAt    *time.Time `json:"last_telemetry_at"`
	TelemetryFreshness string     `json:"telemetry_freshness"`
	PredictionAt       time.Time  `json:"prediction_generated_at"`
	ModelVersion       string     `json:"model_version"`
	WorkOrderID        *int       `json:"work_order_id"`
}

// buildPredictions loads the enriched watchlist (shared by list + summary).
func buildPredictions(ctx context.Context, db *sql.DB) ([]prediction, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT l.id, COALESCE(l.reference, l.id::text) AS reference,
		       COALESCE(l.zone, 'Sans zone') AS zone, COALESCE(l.lcu_reference, '') AS lcu,
		       l.etat, l.fault_status, l.last_seen_at,
		       COALESCE(fc.n, 0) AS fault_count, fc.last_at, COALESCE(fc.avg_conf, 0) AS avg_conf,
		       (SELECT wo.id FROM work_orders wo
		        WHERE wo.lampadaire_id = l.id AND wo.status NOT IN ('closed','cancelled','resolved')
		        ORDER BY wo.created_at DESC LIMIT 1) AS wo_id
		FROM lampadaires l
		LEFT JOIN (
			SELECT lampadaire_id, COUNT(*) AS n, MAX(created_at) AS last_at, AVG(confidence) AS avg_conf
			FROM fault_events GROUP BY lampadaire_id
		) fc ON fc.lampadaire_id = l.id
		WHERE l.archived_at IS NULL AND l.fault_status IS NOT NULL AND l.fault_status <> 'none'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type raw struct {
		p         prediction
		lastSeen  *time.Time
		lastFault *time.Time
		avgConf   float64
	}
	var items []raw
	maxFC := 0
	for rows.Next() {
		var r raw
		var etat string
		var lastSeen, lastFault sql.NullTime
		var woID sql.NullInt64
		if err := rows.Scan(&r.p.ID, &r.p.Reference, &r.p.Zone, &r.p.LCUReference,
			&etat, &r.p.FaultStatus, &lastSeen, &r.p.FaultCount, &lastFault, &r.avgConf, &woID); err != nil {
			continue
		}
		r.p.Online = etat == "online"
		r.p.PredictedLabel = faultLabels[r.p.FaultStatus]
		if r.p.PredictedLabel == "" {
			r.p.PredictedLabel = r.p.FaultStatus
		}
		if lastSeen.Valid {
			r.lastSeen = &lastSeen.Time
		}
		if lastFault.Valid {
			r.lastFault = &lastFault.Time
		}
		if woID.Valid {
			v := int(woID.Int64)
			r.p.WorkOrderID = &v
		}
		if r.p.FaultCount > maxFC {
			maxFC = r.p.FaultCount
		}
		items = append(items, r)
	}

	now := time.Now()
	out := make([]prediction, 0, len(items))
	for _, r := range items {
		p := r.p
		p.RiskScore = computeRiskScore(p.FaultStatus, p.FaultCount, maxFC, r.lastFault)
		p.RiskLevel = riskLevel(p.RiskScore, true)
		p.TelemetryFreshness = telemetryFreshness(r.lastSeen)
		p.Confidence = adjustedConfidence(r.avgConf, p.TelemetryFreshness)
		p.ETAHours, p.ETALabel = etaFromScore(p.RiskScore)
		p.LastTelemetryAt = r.lastSeen
		p.PredictionAt = now
		p.ModelVersion = faultModelVersion
		out = append(out, p)
	}
	return out, nil
}

// HandleGetPredictions handles GET /api/faults/predictions — enriched watchlist.
func HandleGetPredictions(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		preds, err := buildPredictions(c.Request.Context(), db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur calcul prédictions")
			return
		}
		RespondJSON(c, http.StatusOK, preds)
	}
}

// HandleGetPredictiveSummary handles GET /api/faults/predictive-summary.
func HandleGetPredictiveSummary(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		preds, err := buildPredictions(ctx, db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur synthèse prédictive")
			return
		}

		var total, missing int
		_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM lampadaires WHERE archived_at IS NULL`).Scan(&total)
		_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM lampadaires WHERE archived_at IS NULL AND last_seen_at IS NULL`).Scan(&missing)

		var critical, high, moderate, predicted30, priority, createdWO, stale int
		var confSum float64
		for _, p := range preds {
			switch p.RiskLevel {
			case "critical":
				critical++
			case "high":
				high++
			case "moderate":
				moderate++
			}
			if p.ETAHours > 0 && p.ETAHours <= 720 {
				predicted30++
			}
			if p.RiskLevel == "critical" || p.RiskLevel == "high" {
				priority++
			}
			if p.WorkOrderID != nil {
				createdWO++
			}
			if p.TelemetryFreshness == "stale" || p.TelemetryFreshness == "obsolete" {
				stale++
			}
			confSum += p.Confidence
		}
		avgConf := 0.0
		if len(preds) > 0 {
			avgConf = math.Round(confSum / float64(len(preds)))
		}
		atRisk := len(preds)
		healthy := total - atRisk
		if healthy < 0 {
			healthy = 0
		}
		dataQuality := 100.0
		if total > 0 {
			dataQuality = math.Round(float64(total-missing-stale) / float64(total) * 100)
			if dataQuality < 0 {
				dataQuality = 0
			}
		}

		RespondJSON(c, http.StatusOK, gin.H{
			"total_lamp_posts":         total,
			"at_risk_count":            atRisk,
			"critical_count":           critical,
			"high_risk_count":          high,
			"moderate_risk_count":      moderate,
			"healthy_count":            healthy,
			"predicted_failures_30d":   predicted30,
			"average_model_confidence": avgConf,
			"priority_interventions":   priority,
			"created_work_orders":      createdWO,
			"stale_telemetry_count":    stale,
			"missing_telemetry_count":  missing,
			"data_quality_score":       dataQuality,
			"model_version":            faultModelVersion,
			"generated_at":             time.Now(),
		})
	}
}

// HandleGetRiskTrend handles GET /api/faults/trend?days=N.
// Daily anomaly counts by severity level (proxy for risk evolution).
func HandleGetRiskTrend(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		days := 30
		if d, err := strconv.Atoi(c.Query("days")); err == nil && d > 0 && d <= 365 {
			days = d
		}
		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT DATE(created_at)::text AS day,
			       SUM(CASE WHEN fault_type IN (1,4) THEN 1 ELSE 0 END) AS critical,
			       SUM(CASE WHEN fault_type = 2 THEN 1 ELSE 0 END) AS high,
			       SUM(CASE WHEN fault_type = 3 THEN 1 ELSE 0 END) AS moderate
			FROM fault_events
			WHERE created_at >= NOW() - ($1 * INTERVAL '1 day')
			GROUP BY DATE(created_at) ORDER BY day`, days)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur tendance")
			return
		}
		defer rows.Close()

		type point struct {
			Day      string `json:"day"`
			Critical int    `json:"critical"`
			High     int    `json:"high"`
			Moderate int    `json:"moderate"`
		}
		out := []point{}
		for rows.Next() {
			var p point
			if rows.Scan(&p.Day, &p.Critical, &p.High, &p.Moderate) == nil {
				out = append(out, p)
			}
		}
		RespondJSON(c, http.StatusOK, out)
	}
}

// HandleGetLampPrediction handles GET /api/lampadaires/:id/prediction.
// Returns explanatory signals (measured vs expected) + recommendation.
func HandleGetLampPrediction(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := ParseIDParam(c, "id")
		if err != nil {
			RespondError(c, http.StatusBadRequest, "Identifiant invalide")
			return
		}
		ctx := c.Request.Context()

		var ref, zone, lcu, etat, faultStatus string
		var nominalW, intensite sql.NullInt64
		var lastSeen sql.NullTime
		err = db.QueryRowContext(ctx, `
			SELECT COALESCE(reference, id::text), COALESCE(zone,'Sans zone'), COALESCE(lcu_reference,''),
			       etat, COALESCE(fault_status,'none'), nominal_power_w, intensite, last_seen_at
			FROM lampadaires WHERE id = $1 AND archived_at IS NULL`, id).
			Scan(&ref, &zone, &lcu, &etat, &faultStatus, &nominalW, &intensite, &lastSeen)
		if err != nil {
			RespondError(c, http.StatusNotFound, "Lampadaire introuvable")
			return
		}

		// Latest telemetry sample.
		var temp, tension, courant, puissance sql.NullFloat64
		var telAt sql.NullTime
		_ = db.QueryRowContext(ctx, `
			SELECT temperature, tension, courant, puissance, created_at
			FROM sensor_measurements WHERE lampadaire_id = $1
			ORDER BY created_at DESC LIMIT 1`, id).Scan(&temp, &tension, &courant, &puissance, &telAt)

		// Fault stats for score/confidence.
		var faultCount int
		var avgConf sql.NullFloat64
		var lastFault sql.NullTime
		_ = db.QueryRowContext(ctx, `
			SELECT COUNT(*), AVG(confidence), MAX(created_at) FROM fault_events WHERE lampadaire_id = $1`, id).
			Scan(&faultCount, &avgConf, &lastFault)

		var lastFaultPtr, lastSeenPtr *time.Time
		if lastFault.Valid {
			lastFaultPtr = &lastFault.Time
		}
		if lastSeen.Valid {
			lastSeenPtr = &lastSeen.Time
		}
		score := computeRiskScore(faultStatus, faultCount, faultCount, lastFaultPtr)
		fresh := telemetryFreshness(lastSeenPtr)
		conf := adjustedConfidence(nullFloat(avgConf), fresh)
		etaH, etaLabel := etaFromScore(score)

		signals := buildSignals(faultStatus, nullFloat(temp), nullFloat(tension), nullFloat(courant),
			nullFloat(puissance), nullInt(nominalW), nullInt(intensite))

		var telAtPtr *time.Time
		if telAt.Valid {
			telAtPtr = &telAt.Time
		}

		RespondJSON(c, http.StatusOK, gin.H{
			"id":                      id,
			"reference":               ref,
			"zone":                    zone,
			"lcu_reference":           lcu,
			"online":                  etat == "online",
			"fault_status":            faultStatus,
			"predicted_label":         faultLabels[faultStatus],
			"risk_score":              score,
			"risk_level":              riskLevel(score, faultStatus != "none"),
			"confidence":              conf,
			"eta_hours":               etaH,
			"eta_label":               etaLabel,
			"last_telemetry_at":       telAtPtr,
			"telemetry_freshness":     fresh,
			"prediction_generated_at": time.Now(),
			"model_version":           faultModelVersion,
			"signals":                 signals,
			"recommendation":          faultRecommendation(faultStatus),
		})
	}
}

func nullFloat(v sql.NullFloat64) float64 {
	if v.Valid {
		return v.Float64
	}
	return 0
}
func nullInt(v sql.NullInt64) int {
	if v.Valid {
		return int(v.Int64)
	}
	return 0
}

type signal struct {
	Key           string  `json:"key"`
	Label         string  `json:"label"`
	CurrentValue  float64 `json:"current_value"`
	ExpectedValue float64 `json:"expected_value"`
	Unit          string  `json:"unit"`
	DeviationPct  float64 `json:"deviation_percent"`
	Contribution  float64 `json:"contribution_percent"`
	Severity      string  `json:"severity"`
}

// buildSignals derives explanatory signals from the latest telemetry vs expected.
func buildSignals(faultStatus string, temp, tension, courant, puissance float64, nominalW, intensite int) []signal {
	sigs := []signal{}
	expectedPower := 0.0
	if nominalW > 0 && intensite > 0 {
		expectedPower = float64(nominalW) * float64(intensite) / 100.0
	}

	add := func(key, label string, cur, exp float64, unit string, warnAbove, critAbove float64, matches bool) {
		dev := 0.0
		if exp != 0 {
			dev = math.Round((cur - exp) / exp * 100)
		}
		sev := "normal"
		if math.Abs(dev) >= critAbove {
			sev = "critical"
		} else if math.Abs(dev) >= warnAbove {
			sev = "warning"
		}
		contribution := 12.0
		if matches {
			contribution = 45.0
		} else if sev != "normal" {
			contribution = 22.0
		}
		sigs = append(sigs, signal{
			Key: key, Label: label, CurrentValue: math.Round(cur*100) / 100,
			ExpectedValue: math.Round(exp*100) / 100, Unit: unit,
			DeviationPct: dev, Contribution: contribution, Severity: sev,
		})
	}

	if expectedPower > 0 {
		add("power", "Puissance consommée", puissance, expectedPower, "W", 15, 30, faultStatus == "underpower")
	}
	add("current", "Courant", courant, 2.5, "A", 40, 80, faultStatus == "overcurrent")
	add("voltage", "Tension", tension, 230, "V", 3, 8, faultStatus == "overvoltage")
	add("temperature", "Température module", temp, 45, "°C", 40, 65, faultStatus == "overtemp")
	return sigs
}

func faultRecommendation(faultStatus string) string {
	switch faultStatus {
	case "overcurrent":
		return "Vérifier le driver LED et le câblage — risque de court-circuit partiel. Intervention prioritaire."
	case "overvoltage":
		return "Contrôler la tension d'alimentation réseau ; surveiller l'état des condensateurs du driver."
	case "underpower":
		return "Inspecter le module LED et le driver (déplétion probable) ; planifier un remplacement préventif."
	case "leakage":
		return "Contrôler l'isolation et la mise à la terre du luminaire — risque électrique, surtout par temps humide."
	case "overtemp":
		return "Vérifier la ventilation et l'état thermique du driver ; réduire l'intensité si nécessaire."
	default:
		return "Vérifier le driver LED, les connexions électriques, le module LED et l'état thermique du luminaire."
	}
}
