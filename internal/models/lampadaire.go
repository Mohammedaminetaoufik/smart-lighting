package models

// Lampadaire represents a street light in the system.
type Lampadaire struct {
	ID               int      `json:"id"`
	Reference        string   `json:"reference"`
	Latitude         *float64 `json:"latitude,omitempty"`
	Longitude        *float64 `json:"longitude,omitempty"`
	Zone             string   `json:"zone,omitempty"`
	TypeDriver       string   `json:"type_driver,omitempty"`
	Protocole        string   `json:"protocole,omitempty"`
	Puissance        *int     `json:"puissance,omitempty"`
	Etat             string   `json:"etat"`
	Intensite        int      `json:"intensite"`
	DateInstallation *string  `json:"date_installation,omitempty"`
	ArchivedAt       *string  `json:"archived_at,omitempty"`
	LastSeenAt       *string  `json:"last_seen_at,omitempty"`
	LastCommandAt    *string  `json:"last_command_at,omitempty"`
	Address          string   `json:"address,omitempty"`
	Quartier         string   `json:"quartier,omitempty"`
	LCUReference     string   `json:"lcu_reference,omitempty"`
	DriverReference  string   `json:"driver_reference,omitempty"`
	Notes            string   `json:"notes,omitempty"`
	HasCriticalAlert bool     `json:"has_critical_alert"`

	LCUID               *int   `json:"lcu_id,omitempty"`
	DeviceUID           string `json:"device_uid,omitempty"`
	NodeAddress         string `json:"node_address,omitempty"`
	DiscoveredByLCU     bool   `json:"discovered_by_lcu"`
	LocationStatus      string `json:"location_status"`
	CommissioningStatus string `json:"commissioning_status"`

	// Controller fields — embedded from LCU sync, not manually entered
	ControllerUID           string  `json:"controller_uid,omitempty"`
	ControllerType          string  `json:"controller_type,omitempty"`
	ControllerStatus        string  `json:"controller_status,omitempty"`
	ControllerSignalQuality *int    `json:"controller_signal_quality,omitempty"`
	ControllerFirmware      string  `json:"controller_firmware,omitempty"`
	ControllerLastSeenAt    *string `json:"controller_last_seen_at,omitempty"`
	ControllerEmbedded      bool    `json:"controller_embedded"`
	DimmingEnabled          bool    `json:"dimming_enabled"`
	MeteringEnabled         bool    `json:"metering_enabled"`
	ArmoireReference        string  `json:"armoire_reference,omitempty"`
	CircuitReference        string  `json:"circuit_reference,omitempty"`
	CabinetID               *int    `json:"cabinet_id,omitempty"`
	CircuitID               *int    `json:"circuit_id,omitempty"`
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

	LCUID               string
	DeviceUID           string
	NodeAddress         string
	LocationStatus      string
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

	LCUs     []LCU
	LCUForm  LCUFormData
	LCUsJSON interface{}
}
