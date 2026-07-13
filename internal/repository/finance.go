package repository

import (
	"context"
	"database/sql"
	"strconv"

	"map-interactif/internal/models"
)

// kwhExpr is the shared convention for computing kWh from telemetry: use the
// measured `energie` sum when present, otherwise estimate from power assuming a
// 5-minute sampling interval (same convention as the energy dashboards).
const kwhExpr = `COALESCE(NULLIF(SUM(sm.energie), 0), SUM(sm.puissance) * 5.0 / 60.0 / 1000.0)`

// GetTariffConfig returns the full pricing configuration (bands + hour map + globals).
func GetTariffConfig(ctx context.Context, db *sql.DB) (*models.TariffConfig, error) {
	cfg := &models.TariffConfig{
		Tariffs:  []models.EnergyTariff{},
		HourMap:  map[string]string{},
		Currency: "DH",
	}

	rows, err := db.QueryContext(ctx, `
		SELECT period_key, label, price_dh_per_kwh, color, sort_order
		FROM energy_tariffs ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t models.EnergyTariff
		if err := rows.Scan(&t.PeriodKey, &t.Label, &t.PriceDH, &t.Color, &t.SortOrder); err == nil {
			cfg.Tariffs = append(cfg.Tariffs, t)
		}
	}

	hRows, err := db.QueryContext(ctx, `SELECT hour, period_key FROM energy_tariff_hours ORDER BY hour`)
	if err != nil {
		return nil, err
	}
	defer hRows.Close()
	for hRows.Next() {
		var h int
		var key string
		if err := hRows.Scan(&h, &key); err == nil {
			cfg.HourMap[strconv.Itoa(h)] = key
		}
	}

	cfg.Co2Factor = getSettingFloat(ctx, db, "co2_factor_kg_per_kwh", 0.70)
	if v := getSettingString(ctx, db, "currency"); v != "" {
		cfg.Currency = v
	}
	return cfg, nil
}

// UpdateTariffConfig persists new prices and hour mapping (admin action).
func UpdateTariffConfig(ctx context.Context, db *sql.DB, cfg *models.TariffConfig) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, t := range cfg.Tariffs {
		if _, err := tx.ExecContext(ctx,
			`UPDATE energy_tariffs SET price_dh_per_kwh=$1, label=$2, color=$3, updated_at=NOW() WHERE period_key=$4`,
			t.PriceDH, t.Label, t.Color, t.PeriodKey); err != nil {
			return err
		}
	}
	for hourStr, key := range cfg.HourMap {
		h, convErr := strconv.Atoi(hourStr)
		if convErr != nil || h < 0 || h > 23 {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE energy_tariff_hours SET period_key=$1 WHERE hour=$2`, key, h); err != nil {
			return err
		}
	}
	if cfg.Co2Factor > 0 {
		_, _ = tx.ExecContext(ctx,
			`INSERT INTO system_settings (key, value) VALUES ('co2_factor_kg_per_kwh', $1)
			 ON CONFLICT (key) DO UPDATE SET value=$1, updated_at=NOW()`,
			strconv.FormatFloat(cfg.Co2Factor, 'f', -1, 64))
	}
	return tx.Commit()
}

// GetEnergyBill computes the invoice over the last `days` days, broken down by
// ONEE time-of-use band, using measured kWh with power-based fallback.
func GetEnergyBill(ctx context.Context, db *sql.DB, days int) (*models.EnergyBill, error) {
	cfg, err := GetTariffConfig(ctx, db)
	if err != nil {
		return nil, err
	}
	bill := &models.EnergyBill{
		Days:     days,
		Currency: cfg.Currency,
		Lines:    []models.BillPeriodLine{},
	}

	rows, err := db.QueryContext(ctx, `
		SELECT t.period_key, t.label, t.color, t.price_dh_per_kwh,
		       `+kwhExpr+` AS kwh
		FROM sensor_measurements sm
		JOIN energy_tariff_hours th ON th.hour = EXTRACT(HOUR FROM sm.created_at)::int
		JOIN energy_tariffs t       ON t.period_key = th.period_key
		WHERE sm.created_at >= NOW() - ($1 * INTERVAL '1 day')
		  AND (sm.energie IS NOT NULL OR sm.puissance IS NOT NULL)
		GROUP BY t.period_key, t.label, t.color, t.price_dh_per_kwh, t.sort_order
		ORDER BY t.sort_order`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var l models.BillPeriodLine
		if err := rows.Scan(&l.PeriodKey, &l.Label, &l.Color, &l.PriceDH, &l.KWh); err == nil {
			l.CostDH = l.KWh * l.PriceDH
			bill.TotalKWh += l.KWh
			bill.TotalCostDH += l.CostDH
			bill.Lines = append(bill.Lines, l)
		}
	}

	// Part réellement mesurée vs estimée (transparence pour la direction).
	var measured, estimated float64
	_ = db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(energie), 0),
		       COALESCE(SUM(CASE WHEN energie IS NULL THEN puissance * 5.0/60.0/1000.0 ELSE 0 END), 0)
		FROM sensor_measurements
		WHERE created_at >= NOW() - ($1 * INTERVAL '1 day')
		  AND (energie IS NOT NULL OR puissance IS NOT NULL)`, days).Scan(&measured, &estimated)
	if measured+estimated > 0 {
		bill.MeasuredShare = measured / (measured + estimated)
	}

	bill.Co2Kg = bill.TotalKWh * cfg.Co2Factor
	return bill, nil
}

// GetFinancialSummary returns the compact KPI block: today's/month's cost in DH,
// estimated savings from dimming, yearly projection and avoided CO2.
func GetFinancialSummary(ctx context.Context, db *sql.DB) (*models.FinancialSummary, error) {
	cfg, err := GetTariffConfig(ctx, db)
	if err != nil {
		return nil, err
	}
	s := &models.FinancialSummary{Currency: cfg.Currency}

	todayBill, err := GetEnergyBill(ctx, db, 1)
	if err != nil {
		return nil, err
	}
	s.CostTodayDH = todayBill.TotalCostDH

	monthBill, err := GetEnergyBill(ctx, db, 30)
	if err != nil {
		return nil, err
	}
	s.CostMonthDH = monthBill.TotalCostDH
	s.ProjectedYearDH = monthBill.TotalCostDH * 12

	// Économie estimée du dimming : approximation linéaire (conso ∝ intensité),
	// cohérente avec les autres calculs du produit. Si l'intensité moyenne est d%,
	// alors la conso pleine puissance ≈ réelle/(d/100), et l'économie ≈ réelle×(100-d)/d.
	var avgIntensity float64
	_ = db.QueryRowContext(ctx, `
		SELECT COALESCE(AVG(intensite), 0) FROM lampadaires
		WHERE archived_at IS NULL AND etat != 'offline' AND intensite > 0`).Scan(&avgIntensity)

	if avgIntensity > 0 {
		factor := (100.0 - avgIntensity) / avgIntensity
		s.SavingMonthDH = monthBill.TotalCostDH * factor
		s.SavingPercent = 100.0 - avgIntensity
		savedKWhMonth := monthBill.TotalKWh * factor
		s.Co2AvoidedKgMonth = savedKWhMonth * cfg.Co2Factor
	}
	return s, nil
}

// getSettingFloat reads a numeric system_setting with a default fallback.
func getSettingFloat(ctx context.Context, db *sql.DB, key string, def float64) float64 {
	var raw string
	if err := db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE key=$1`, key).Scan(&raw); err != nil {
		return def
	}
	if v, err := strconv.ParseFloat(raw, 64); err == nil {
		return v
	}
	return def
}

// getSettingString reads a text system_setting ("" if absent).
func getSettingString(ctx context.Context, db *sql.DB, key string) string {
	var raw string
	if err := db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE key=$1`, key).Scan(&raw); err != nil {
		return ""
	}
	return raw
}
