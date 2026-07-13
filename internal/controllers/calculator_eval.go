package controllers

import (
	"database/sql"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/services"
)

// representativeLux returns a lux value inside each bucket so the lookup
// re-classifies into the intended regime.
func representativeLux(bucket string) float64 {
	switch bucket {
	case "nuit":
		return 5
	case "crepuscule":
		return 50
	default:
		return 500
	}
}

// HandleEvaluateDataset handles GET /api/calculator/evaluate-dataset (admin).
// Compares the data-driven dimming profile against the dataset ground truth
// (dimming_reference), and quantifies the energy saving vs a fixed-100% lamp.
func HandleEvaluateDataset(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Recharge la référence pour refléter un éventuel import récent.
		_ = services.LoadDimmingReference(c.Request.Context(), db)
		if !services.DimmingReferenceReady() {
			RespondError(c, http.StatusServiceUnavailable,
				"Aucun profil de dimming appris. Lancez d'abord tools/import-telemetry.")
			return
		}

		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT hour, lux_bucket, presence, recommended_brightness, sample_count
			FROM dimming_reference`)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lecture référence")
			return
		}
		defer rows.Close()

		var (
			totalW, maeW, concordW  float64
			litW, savingW           float64
			conditions              int
		)
		for rows.Next() {
			var (
				hour     int
				bucket   string
				presence bool
				truth    float64
				count    int
			)
			if err := rows.Scan(&hour, &bucket, &presence, &truth, &count); err != nil {
				continue
			}
			w := float64(count)
			pred, ok := services.LookupRecommendedBrightness(hour, representativeLux(bucket), presence)
			if !ok {
				continue
			}
			conditions++
			diff := math.Abs(float64(pred) - truth)
			totalW += w
			maeW += w * diff
			if diff <= 10 {
				concordW += w
			}
			// Économie vs éclairage fixe 100 % (seulement là où on éclaire).
			if truth > 0 {
				litW += w
				savingW += w * (100 - truth)
			}
		}

		if totalW == 0 {
			RespondError(c, http.StatusServiceUnavailable, "Référence vide")
			return
		}

		resp := gin.H{
			"conditions_evaluated":    conditions,
			"samples":                 int(totalW),
			"mae_brightness_pct":      round1(maeW / totalW),
			"concordance_pct":         round1(concordW / totalW * 100),
			"avg_saving_vs_full_pct":  0.0,
			"interpretation": "MAE = écart moyen (points de %) entre le profil appris et le dataset. " +
				"Concordance = part des cas à ±10 pts. Économie = réduction moyenne d'intensité vs un éclairage fixe à 100 %.",
		}
		if litW > 0 {
			resp["avg_saving_vs_full_pct"] = round1(savingW / litW)
		}
		RespondJSON(c, http.StatusOK, resp)
	}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
