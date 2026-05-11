package main

import "time"

// Lampadaire represents a street light in the system.
type Lampadaire struct {
	ID               int     `json:"id"`
	Reference        string  `json:"reference"`
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
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
	DiscoveredByLCU bool   `json:"discovered_by_lcu"`
	LocationStatus  string `json:"location_status"`
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

// Alert represents a system alert related to a lampadaire.
type Alert struct {
	ID           int        `json:"id"`
	LampadaireID *int       `json:"lampadaire_id,omitempty"`
	Type         string     `json:"type"`
	Severity     string     `json:"severity"`
	Message      string     `json:"message"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	Reference    string     `json:"reference,omitempty"`
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
	MissingLocation        int                 `json:"missing_location"`
	OpenAlerts             int                 `json:"open_alerts"`
	CriticalAlerts         int                 `json:"critical_alerts"`
	AvgIntensity           float64             `json:"avg_intensity"`
	AvgPower               float64             `json:"avg_power"`
	EstimatedPowerSavingW  float64             `json:"estimated_power_saving_w"`
	EstimatedSavingPercent float64             `json:"estimated_saving_percent"`
	RecentAlerts           []Alert             `json:"recent_alerts"`
	RecentCommands         []DimmingCommand    `json:"recent_commands"`
	RecentTelemetry        []SensorMeasurement `json:"recent_telemetry"`
}

type User struct {
	ID        int
	FullName  string
	Email     string
	Role      string
	Status    string
	CreatedAt string
}

type Intervention struct {
	ID           int
	AlertID      *int
	LampadaireID *int
	AssignedTo   *int
	Title        string
	Description  string
	Priority     string
	Status       string
	CreatedAt    string
	ClosedAt     *string
}

type SystemSetting struct {
	Key   string
	Value string
}

type AccessLog struct {
ID int `json:"id"`
UserID int `json:"user_id"`
Action string `json:"action"`
CreatedAt string `json:"created_at"`
}

