package models

// EnergyTariff is one ONEE time-of-use pricing band.
type EnergyTariff struct {
	PeriodKey string  `json:"period_key"` // peak | full | off
	Label     string  `json:"label"`
	PriceDH   float64 `json:"price_dh_per_kwh"`
	Color     string  `json:"color"`
	SortOrder int     `json:"sort_order"`
}

// TariffConfig is the full pricing configuration: bands + hour→band map + globals.
type TariffConfig struct {
	Tariffs   []EnergyTariff `json:"tariffs"`
	HourMap   map[string]string `json:"hour_map"` // "0".."23" → period_key
	Co2Factor float64        `json:"co2_factor_kg_per_kwh"`
	Currency  string         `json:"currency"`
}

// BillPeriodLine is the cost breakdown for one pricing band.
type BillPeriodLine struct {
	PeriodKey string  `json:"period_key"`
	Label     string  `json:"label"`
	Color     string  `json:"color"`
	KWh       float64 `json:"kwh"`
	PriceDH   float64 `json:"price_dh_per_kwh"`
	CostDH    float64 `json:"cost_dh"`
}

// EnergyBill is the computed invoice over a period.
type EnergyBill struct {
	Days          int              `json:"days"`
	Currency      string           `json:"currency"`
	TotalKWh      float64          `json:"total_kwh"`
	TotalCostDH   float64          `json:"total_cost_dh"`
	Lines         []BillPeriodLine `json:"lines"`
	MeasuredShare float64          `json:"measured_share"` // 0..1 part réellement mesurée
	Co2Kg         float64          `json:"co2_kg"`
}

// FinancialSummary is the compact KPI block for the dashboard/direction.
type FinancialSummary struct {
	Currency          string  `json:"currency"`
	CostTodayDH       float64 `json:"cost_today_dh"`
	CostMonthDH       float64 `json:"cost_month_dh"`
	SavingMonthDH     float64 `json:"saving_month_dh"`
	SavingPercent     float64 `json:"saving_percent"`
	ProjectedYearDH   float64 `json:"projected_year_dh"`
	Co2AvoidedKgMonth float64 `json:"co2_avoided_kg_month"`
}
