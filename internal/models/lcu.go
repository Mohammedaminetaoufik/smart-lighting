package models

import "time"

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

	// Controller info returned by the LCU during device discovery
	ControllerType          string `json:"controller_type,omitempty"`
	ControllerFirmware      string `json:"controller_firmware,omitempty"`
	ControllerSignalQuality *int   `json:"controller_signal_quality,omitempty"`
	ControllerEmbedded      bool   `json:"controller_embedded"`
	DimmingEnabled          bool   `json:"dimming_enabled"`
	MeteringEnabled         bool   `json:"metering_enabled"`
	ArmoireReference        string `json:"armoire_reference,omitempty"`
	CircuitReference        string `json:"circuit_reference,omitempty"`
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
