package main

import "time"

// Lampadaire represents a street light in the system.
type Lampadaire struct {
	ID               int      `json:"id"`
	Reference        string   `json:"reference"`
	Latitude         *float64 `json:"latitude,omitempty"`
	Longitude        *float64 `json:"longitude,omitempty"`
	Zone             string  `json:"zone,omitempty"`
	TypeDriver       string  `json:"type_driver,omitempty"`
	Protocole        string  `json:"protocole,omitempty"`
	Puissance        *int    `json:"puissance,omitempty"`
	Etat             string  `json:"etat"`
	Intensite        int     `json:"intensite"`
	DateInstallation *string `json:"date_installation,omitempty"`
	ArchivedAt       *string `json:"archived_at,omitempty"`
	LastSeenAt       *string `json:"last_seen_at,omitempty"`
	LastCommandAt    *string `json:"last_command_at,omitempty"`
	Address          string  `json:"address,omitempty"`
	Quartier         string  `json:"quartier,omitempty"`
	LCUReference     string  `json:"lcu_reference,omitempty"`
	DriverReference  string  `json:"driver_reference,omitempty"`
	Notes            string  `json:"notes,omitempty"`
	HasCriticalAlert bool    `json:"has_critical_alert"`

	// New fields for professional IoT logic
	LCUID           *int   `json:"lcu_id,omitempty"`
	DeviceUID       string `json:"device_uid,omitempty"`
	NodeAddress     string `json:"node_address,omitempty"`
	DiscoveredByLCU     bool   `json:"discovered_by_lcu"`
	LocationStatus      string `json:"location_status"`
	CommissioningStatus string `json:"commissioning_status"`
}

// LCU represents a Local Control Unit / Gateway.
type LCU struct {
	ID         int        `json:"id"`
	Reference  string     `json:"reference"`
	Name       string     `json:"name,omitempty"`
	IPAddress  string     `json:"ip_address"`
	Port       int        `json:"port"`
	Protocol   string     `json:"protocol"`
	AuthToken  string     `json:"-"`
	Zone       string     `json:"zone,omitempty"`
	Address    string     `json:"address,omitempty"`
	Latitude   *float64   `json:"latitude,omitempty"`
	Longitude  *float64   `json:"longitude,omitempty"`
	Status     string     `json:"status"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	LastSyncAt *time.Time `json:"last_sync_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// LCUFormData holds HTML form input for creating/editing an LCU.
type LCUFormData struct {
	Reference string
	Name      string
	IPAddress string
	Port      string
	Protocol  string
	AuthToken string
	Zone      string
	Address   string
	Latitude  string
	Longitude string
}

// LcuDeviceDTO is the data transfer object for devices discovered by an LCU.
type LcuDeviceDTO struct {
	DeviceUID   string   `json:"device_uid"`
	Reference   string   `json:"reference"`
	NodeAddress string   `json:"node_address"`
	Latitude    *float64 `json:"latitude,omitempty"`
	Longitude   *float64 `json:"longitude,omitempty"`
	Zone        string   `json:"zone,omitempty"`
	TypeDriver  string   `json:"type_driver,omitempty"`
	Protocole   string   `json:"protocole,omitempty"`
	Puissance   *int     `json:"puissance,omitempty"`
	Etat        string   `json:"etat"`
	Intensite   int      `json:"intensite"`
}

// LCUSyncLog represents a log entry for an LCU synchronization process.
type LCUSyncLog struct {
	ID              int       `json:"id"`
	LCUID           int       `json:"lcu_id"`
	Status          string    `json:"status"`
	Message         string    `json:"message"`
	DiscoveredCount int       `json:"discovered_count"`
	CreatedCount    int       `json:"created_count"`
	UpdatedCount    int       `json:"updated_count"`
	FailedCount     int       `json:"failed_count"`
	CreatedAt       time.Time `json:"created_at"`
}

// SensorMeasurement represents one telemetry reading from a lampadaire's sensors.
type SensorMeasurement struct {
	ID           int       `json:"id"`
	LampadaireID int       `json:"lampadaire_id"`
	Luminosite   *float64  `json:"luminosite,omitempty"`
	Presence     *bool     `json:"presence,omitempty"`
	Temperature  *float64  `json:"temperature,omitempty"`
	Humidite     *float64  `json:"humidite,omitempty"`
	Tension      *float64  `json:"tension,omitempty"`
	Courant      *float64  `json:"courant,omitempty"`
	Puissance    *float64  `json:"puissance,omitempty"`
	Energie      *float64  `json:"energie,omitempty"`
	Source       string    `json:"source"`
	CreatedAt    time.Time `json:"created_at"`

	// Optional fields for Format 2 telemetry
	LCUReference string `json:"lcu_reference,omitempty"`
	DeviceUID    string `json:"device_uid,omitempty"`
}

// DimmingCommand represents a dimming order sent to a lampadaire.
type DimmingCommand struct {
	ID           int        `json:"id"`
	LampadaireID int        `json:"lampadaire_id"`
	Source       string     `json:"source"`
	OldIntensity *int       `json:"old_intensity,omitempty"`
	NewIntensity int        `json:"new_intensity"`
	Reason       string     `json:"reason,omitempty"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	AppliedAt    *time.Time `json:"applied_at,omitempty"`
}

// Alert represents a system alert related to a lampadaire, cabinet, basestation or circuit.
type Alert struct {
	ID                int        `json:"id"`
	LampadaireID      *int       `json:"lampadaire_id,omitempty"`
	CabinetID         *int       `json:"cabinet_id,omitempty"`
	BasestationID     *int       `json:"basestation_id,omitempty"`
	CircuitID         *int       `json:"circuit_id,omitempty"`
	SourceType        string     `json:"source_type,omitempty"`
	Type              string     `json:"type"`
	Severity          string     `json:"severity"`
	Message           string     `json:"message"`
	Status            string     `json:"status"`
	ProbableCause     string     `json:"probable_cause,omitempty"`
	RecommendedAction string     `json:"recommended_action,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	AcknowledgedAt    *time.Time `json:"acknowledged_at,omitempty"`
	ResolvedAt        *time.Time `json:"resolved_at,omitempty"`
	ClosedAt          *time.Time `json:"closed_at,omitempty"`
	Reference         string     `json:"reference,omitempty"`
}

// CalculatorDecision represents an intelligent calculator recommendation.
type CalculatorDecision struct {
	ID                   int       `json:"id"`
	LampadaireID         int       `json:"lampadaire_id"`
	RecommendedIntensity int       `json:"recommended_intensity"`
	DecisionReason       string    `json:"decision_reason"`
	Confidence           float64   `json:"confidence"`
	Applied              bool      `json:"applied"`
	RuleName             string    `json:"rule_name,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}

// FormData holds HTML form input for creating/editing a lampadaire.
type FormData struct {
	Reference        string
	Latitude         string
	Longitude        string
	Zone             string
	TypeDriver       string
	Protocole        string
	Puissance        string
	Etat             string
	Intensite        string
	DateInstallation string
	Address          string
	Quartier         string
	LCUReference     string
	DriverReference  string
	Notes            string

	// New fields
	LCUID          string
	DeviceUID      string
	NodeAddress    string
	LocationStatus string
	CommissioningStatus string
}

// PageData is the data passed to the main HTML template.
type PageData struct {
	Lampadaires         []Lampadaire
	ArchivedLampadaires []Lampadaire
	LampadairesJSON     interface{}
	Success             bool
	Errors              []string
	Form                FormData

	// New fields for LCU management
	LCUs     []LCU
	LCUForm  LCUFormData
	LCUsJSON interface{}
}

// DashboardStats holds aggregate statistics for the dashboard.
type DashboardStats struct {
	TotalLCUs              int                 `json:"total_lcus"`
	LCUsOnline             int                 `json:"lcus_online"`
	LCUsOffline            int                 `json:"lcus_offline"`
	TotalLampadaires       int                 `json:"total_lampadaires"`
	LampadairesOnline      int                 `json:"lampadaires_online"`
	LampadairesOffline     int                 `json:"lampadaires_offline"`
	LampadairesMaintenance int                 `json:"lampadaires_maintenance"`
	InactiveLampadaires    int                 `json:"inactive_lampadaires"`
	MissingLocation        int                 `json:"missing_location"`
	OpenAlerts             int                 `json:"open_alerts"`
	CriticalAlerts         int                 `json:"critical_alerts"`
	CommandsToday          int                 `json:"commands_today"`
	AvgIntensity           float64             `json:"avg_intensity"`
	AvgPower               float64             `json:"avg_power"`
	EstimatedPowerSavingW  float64             `json:"estimated_power_saving_w"`
	EstimatedSavingPercent float64             `json:"estimated_saving_percent"`
	// New entities
	TotalBasestations       int `json:"total_basestations"`
	BasestationsOnline      int `json:"basestations_online"`
	TotalCabinets           int `json:"total_cabinets"`
	TotalControllers        int `json:"total_controllers"`
	OpenWorkOrders          int `json:"open_work_orders"`
	UrgentWorkOrders        int `json:"urgent_work_orders"`
	CommissioningRate       float64 `json:"commissioning_rate"`
	RecentAlerts            []Alert             `json:"recent_alerts"`
	RecentCommands          []DimmingCommand    `json:"recent_commands"`
	RecentTelemetry         []SensorMeasurement `json:"recent_telemetry"`
}

type User struct {
	ID        int       `json:"id"`
	FullName  string    `json:"full_name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type Intervention struct {
	ID           int        `json:"id"`
	AlertID      *int       `json:"alert_id,omitempty"`
	LampadaireID *int       `json:"lampadaire_id,omitempty"`
	AssignedTo   *int       `json:"assigned_to,omitempty"`
	Title        string     `json:"title"`
	Description  string     `json:"description,omitempty"`
	Priority     string     `json:"priority"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ClosedAt     *time.Time `json:"closed_at,omitempty"`
}

type SystemSetting struct {
	Key   string
	Value string
}

type AccessLog struct {
	ID        int    `json:"id"`
	UserID    int    `json:"user_id"`
	Action    string `json:"action"`
	CreatedAt string `json:"created_at"`
}

type EnergySummary struct {
	TotalNominalPowerW     float64             `json:"total_nominal_power_w"`
	EstimatedCurrentPowerW float64             `json:"estimated_current_power_w"`
	EstimatedSavingW       float64             `json:"estimated_saving_w"`
	EstimatedSavingPercent float64             `json:"estimated_saving_percent"`
	ByZone                 []EnergyZoneSummary `json:"by_zone"`
}

type EnergyZoneSummary struct {
	Zone                   string  `json:"zone"`
	TotalNominalPowerW     float64 `json:"total_nominal_power_w"`
	EstimatedCurrentPowerW float64 `json:"estimated_current_power_w"`
	EstimatedSavingW       float64 `json:"estimated_saving_w"`
	EstimatedSavingPercent float64 `json:"estimated_saving_percent"`
}

// LightingGroup represents a logical collection of lampadaires.
type LightingGroup struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Zone        string    `json:"zone,omitempty"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// LightingProfile represents a dimming strategy for a zone or group.
type LightingProfile struct {
	ID          int                       `json:"id"`
	Name        string                    `json:"name"`
	Description string                    `json:"description,omitempty"`
	TargetType  string                    `json:"target_type"`  // zone, group, lcu
	TargetValue string                    `json:"target_value"` // e.g. "Zone A" or GroupID
	Enabled     bool                      `json:"enabled"`
	Schedules   []LightingProfileSchedule `json:"schedules,omitempty"`
	CreatedAt   time.Time                 `json:"created_at"`
	UpdatedAt   time.Time                 `json:"updated_at"`
}

// LightingProfileSchedule represents a specific timing within a lighting profile.
type LightingProfileSchedule struct {
	ID         int       `json:"id"`
	ProfileID  int       `json:"profile_id"`
	StartTime  string    `json:"start_time"` // HH:MM
	EndTime    string    `json:"end_time"`   // HH:MM
	Intensity  int       `json:"intensity"`
	DaysOfWeek string    `json:"days_of_week,omitempty"` // 1,2,3,4,5,6,7
	CreatedAt  time.Time `json:"created_at"`
}
