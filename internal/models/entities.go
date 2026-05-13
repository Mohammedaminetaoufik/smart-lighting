package models

import "time"

type Basestation struct {
	ID                     int        `json:"id"`
	Reference              string     `json:"reference"`
	Name                   string     `json:"name"`
	Zone                   string     `json:"zone"`
	Address                string     `json:"address"`
	Latitude               *float64   `json:"latitude"`
	Longitude              *float64   `json:"longitude"`
	Status                 string     `json:"status"`
	NetworkType            string     `json:"network_type"`
	PrimaryBackhaul        string     `json:"primary_backhaul"`
	ActiveBackhaul         string     `json:"active_backhaul"`
	ConnectedNodesCount    int        `json:"connected_nodes_count"`
	DisconnectedNodesCount int        `json:"disconnected_nodes_count"`
	SignalQualityAvg       float64    `json:"signal_quality_avg"`
	BatteryStatus          string     `json:"battery_status"`
	LastSeenAt             *time.Time `json:"last_seen_at"`
	CommissionedAt         *time.Time `json:"commissioned_at"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type Cabinet struct {
	ID             int        `json:"id"`
	Reference      string     `json:"reference"`
	Name           string     `json:"name"`
	Zone           string     `json:"zone"`
	Address        string     `json:"address"`
	Latitude       *float64   `json:"latitude"`
	Longitude      *float64   `json:"longitude"`
	Status         string     `json:"status"`
	DoorStatus     string     `json:"door_status"`
	PowerStatus    string     `json:"power_status"`
	VoltageL1      *float64   `json:"voltage_l1"`
	VoltageL2      *float64   `json:"voltage_l2"`
	VoltageL3      *float64   `json:"voltage_l3"`
	CurrentL1      *float64   `json:"current_l1"`
	CurrentL2      *float64   `json:"current_l2"`
	CurrentL3      *float64   `json:"current_l3"`
	LeakageCurrent *float64   `json:"leakage_current"`
	EnergyKwh      float64    `json:"energy_kwh"`
	LastSeenAt     *time.Time `json:"last_seen_at"`
	Notes          string     `json:"notes"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type CabinetCircuit struct {
	ID              int        `json:"id"`
	CabinetID       int        `json:"cabinet_id"`
	Name            string     `json:"name"`
	Phase           string     `json:"phase"`
	CircuitNumber   int        `json:"circuit_number"`
	Status          string     `json:"status"`
	ContactorStatus string     `json:"contactor_status"`
	BreakerStatus   string     `json:"breaker_status"`
	MeasuredCurrent *float64   `json:"measured_current"`
	MeasuredVoltage *float64   `json:"measured_voltage"`
	MeasuredPower   *float64   `json:"measured_power"`
	LampCount       int        `json:"lamp_count"`
	ProfileID       *int       `json:"profile_id"`
	LastFaultAt     *time.Time `json:"last_fault_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Controller struct {
	ID                  int        `json:"id"`
	ControllerUID       string     `json:"controller_uid"`
	SerialNumber        string     `json:"serial_number"`
	Type                string     `json:"type"`
	LampadaireID        *int       `json:"lampadaire_id"`
	BasestationID       *int       `json:"basestation_id"`
	CabinetID           *int       `json:"cabinet_id"`
	FirmwareVersion     string     `json:"firmware_version"`
	CommunicationStatus string     `json:"communication_status"`
	SignalQuality       int        `json:"signal_quality"`
	LastSeenAt          *time.Time `json:"last_seen_at"`
	MeteringEnabled     bool       `json:"metering_enabled"`
	DimmingEnabled      bool       `json:"dimming_enabled"`
	InstallationStatus  string     `json:"installation_status"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type WorkOrder struct {
	ID             int        `json:"id"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	Priority       string     `json:"priority"`
	Status         string     `json:"status"`
	SourceAlertIDs []int      `json:"source_alert_ids,omitempty"`
	LampadaireID   *int       `json:"lampadaire_id"`
	CabinetID      *int       `json:"cabinet_id"`
	BasestationID  *int       `json:"basestation_id"`
	CircuitID      *int       `json:"circuit_id"`
	AssignedTo     *int       `json:"assigned_to"`
	CrewType       string     `json:"crew_type"`
	DueDate        *time.Time `json:"due_date"`
	ProbableCause  string     `json:"probable_cause"`
	ResolutionNote string     `json:"resolution_note"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ClosedAt       *time.Time `json:"closed_at"`
}
