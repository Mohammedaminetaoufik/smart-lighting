package models

import "time"

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
