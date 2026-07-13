package controllers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/models"
	"map-interactif/internal/repository"
)

// HandleGetTariffs handles GET /api/energy/tariffs.
func HandleGetTariffs(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg, err := repository.GetTariffConfig(c.Request.Context(), db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur lecture tarification")
			return
		}
		RespondJSON(c, http.StatusOK, cfg)
	}
}

// HandleUpdateTariffs handles PUT /api/energy/tariffs (admin only).
func HandleUpdateTariffs(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var cfg models.TariffConfig
		if !BindRequiredJSON(c, &cfg) {
			return
		}
		if err := repository.UpdateTariffConfig(c.Request.Context(), db, &cfg); err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur mise à jour tarification")
			return
		}
		updated, err := repository.GetTariffConfig(c.Request.Context(), db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur relecture tarification")
			return
		}
		RespondJSON(c, http.StatusOK, updated)
	}
}

// HandleGetEnergyBill handles GET /api/energy/bill?days=30.
func HandleGetEnergyBill(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		days := 30
		if d, err := strconv.Atoi(c.Query("days")); err == nil && d > 0 && d <= 365 {
			days = d
		}
		bill, err := repository.GetEnergyBill(c.Request.Context(), db, days)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur calcul facture")
			return
		}
		RespondJSON(c, http.StatusOK, bill)
	}
}

// HandleGetFinancialSummary handles GET /api/finance/summary.
func HandleGetFinancialSummary(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		summary, err := repository.GetFinancialSummary(c.Request.Context(), db)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "Erreur synthèse financière")
			return
		}
		RespondJSON(c, http.StatusOK, summary)
	}
}
