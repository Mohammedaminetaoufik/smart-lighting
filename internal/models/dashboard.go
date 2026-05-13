package models

// DashboardStats holds aggregate statistics for the dashboard.
type DashboardStats struct {
	TotalLCUs              int     `json:"total_lcus"`
	LCUsOnline             int     `json:"lcus_online"`
	LCUsOffline            int     `json:"lcus_offline"`
	TotalLampadaires       int     `json:"total_lampadaires"`
	LampadairesOnline      int     `json:"lampadaires_online"`
	LampadairesOffline     int     `json:"lampadaires_offline"`
	LampadairesMaintenance int     `json:"lampadaires_maintenance"`
	InactiveLampadaires    int     `json:"inactive_lampadaires"`
	MissingLocation        int     `json:"missing_location"`
	OpenAlerts             int     `json:"open_alerts"`
	CriticalAlerts         int     `json:"critical_alerts"`
	CommandsToday          int     `json:"commands_today"`
	AvgIntensity           float64 `json:"avg_intensity"`
	AvgPower               float64 `json:"avg_power"`
	EstimatedPowerSavingW  float64 `json:"estimated_power_saving_w"`
	EstimatedSavingPercent float64 `json:"estimated_saving_percent"`
	// New entities
	TotalBasestations  int     `json:"total_basestations"`
	BasestationsOnline int     `json:"basestations_online"`
	TotalCabinets      int     `json:"total_cabinets"`
	TotalControllers   int     `json:"total_controllers"`
	OpenWorkOrders     int     `json:"open_work_orders"`
	UrgentWorkOrders   int     `json:"urgent_work_orders"`
	CommissioningRate  float64 `json:"commissioning_rate"`
	RecentAlerts       []Alert             `json:"recent_alerts"`
	RecentCommands     []DimmingCommand    `json:"recent_commands"`
	RecentTelemetry    []SensorMeasurement `json:"recent_telemetry"`
}

type EnergySummary struct {
	TotalNominalPowerW     float64              `json:"total_nominal_power_w"`
	EstimatedCurrentPowerW float64              `json:"estimated_current_power_w"`
	EstimatedSavingW       float64              `json:"estimated_saving_w"`
	EstimatedSavingPercent float64              `json:"estimated_saving_percent"`
	ByZone                 []EnergyZoneSummary  `json:"by_zone"`
}

type EnergyZoneSummary struct {
	Zone                   string  `json:"zone"`
	TotalNominalPowerW     float64 `json:"total_nominal_power_w"`
	EstimatedCurrentPowerW float64 `json:"estimated_current_power_w"`
	EstimatedSavingW       float64 `json:"estimated_saving_w"`
	EstimatedSavingPercent float64 `json:"estimated_saving_percent"`
}

// NetworkHealth aggregates network connectivity stats.
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

// CommissioningProgress aggregates commissioning progress.
type CommissioningProgress struct {
	Steps          []CommissioningStep `json:"steps"`
	Total          int                 `json:"total"`
	Commissioned   int                 `json:"commissioned"`
	CommissioningRate float64          `json:"commissioning_rate"`
}

type CommissioningStep struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}
