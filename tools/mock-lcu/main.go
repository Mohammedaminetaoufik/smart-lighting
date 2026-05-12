package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Device struct {
	DeviceUID   string   `json:"device_uid"`
	Reference   string   `json:"reference"`
	NodeAddress string   `json:"node_address"`
	Latitude    *float64 `json:"latitude"`
	Longitude   *float64 `json:"longitude"`
	Zone        string   `json:"zone"`
	TypeDriver  string   `json:"type_driver"`
	Protocole   string   `json:"protocole"`
	Puissance   int      `json:"puissance"`
	Etat        string   `json:"etat"`
	Intensite   int      `json:"intensite"`
}

type Telemetry struct {
	DeviceUID   string  `json:"device_uid"`
	Luminosite  float64 `json:"luminosite"`
	Presence    bool    `json:"presence"`
	Temperature float64 `json:"temperature"`
	Humidite    float64 `json:"humidite"`
	Tension     float64 `json:"tension"`
	Courant     float64 `json:"courant"`
	Puissance   float64 `json:"puissance"`
	Energie     float64 `json:"energie"`
	Source      string  `json:"source"`
}

var (
	devices = make(map[string]*Device)
	mu      sync.RWMutex
)

func init() {
	lat1, lng1 := 31.6251, -7.9892
	lat2, lng2 := 31.6255, -7.9887
	lat3, lng3 := 31.6260, -7.9880

	initialDevices := []*Device{
		{DeviceUID: "LCU-TEST-001-LAMP-001", Reference: "LP-001", NodeAddress: "0x01", Latitude: &lat1, Longitude: &lng1, Zone: "Zone A", TypeDriver: "DALI", Protocole: "ZigBee", Puissance: 100, Etat: "online", Intensite: 70},
		{DeviceUID: "LCU-TEST-001-LAMP-002", Reference: "LP-002", NodeAddress: "0x02", Latitude: &lat2, Longitude: &lng2, Zone: "Zone A", TypeDriver: "DALI", Protocole: "ZigBee", Puissance: 120, Etat: "online", Intensite: 60},
		{DeviceUID: "LCU-TEST-001-LAMP-003", Reference: "LP-003", NodeAddress: "0x03", Latitude: &lat3, Longitude: &lng3, Zone: "Zone A", TypeDriver: "0-10V", Protocole: "ZigBee", Puissance: 80, Etat: "maintenance", Intensite: 0},
		{DeviceUID: "LCU-TEST-001-LAMP-004", Reference: "LP-004", NodeAddress: "0x04", Zone: "Zone A", TypeDriver: "DALI", Protocole: "ZigBee", Puissance: 90, Etat: "offline", Intensite: 0},
		{DeviceUID: "LCU-TEST-001-LAMP-005", Reference: "LP-005", NodeAddress: "0x05", Zone: "Zone A", TypeDriver: "PWM", Protocole: "ZigBee", Puissance: 75, Etat: "online", Intensite: 50},
	}

	for _, d := range initialDevices {
		devices[d.DeviceUID] = d
	}
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/devices", handleDevices)
	mux.HandleFunc("/api/devices/", handleDeviceAction)

	port := ":9091"
	fmt.Printf("LCU Virtuelle demarree sur http://localhost%s\n", port)
	log.Fatal(http.ListenAndServe(port, mux))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]string{
		"status":    "online",
		"reference": "LCU-TEST-001",
		"firmware":  "1.0.0",
		"protocol":  "HTTP",
		"time":      time.Now().Format(time.RFC3339),
	}
	json.NewEncoder(w).Encode(resp)
}

func handleDevices(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	defer mu.RUnlock()

	var list []*Device
	for _, d := range devices {
		list = append(list, d)
	}
	json.NewEncoder(w).Encode(list)
}

func handleDeviceAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	uid := parts[3]
	action := ""
	if len(parts) >= 5 {
		action = parts[4]
	}

	mu.RLock()
	device, ok := devices[uid]
	mu.RUnlock()

	if !ok {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	switch action {
	case "dimming":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Intensity int    `json:"intensity"`
			Reason    string `json:"reason"`
			Source    string `json:"source"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		mu.Lock()
		device.Intensite = req.Intensity
		mu.Unlock()

		resp := map[string]interface{}{
			"status":     "applied",
			"device_uid": uid,
			"intensity":  req.Intensity,
			"source":     req.Source,
			"reason":     req.Reason,
		}
		json.NewEncoder(w).Encode(resp)

	case "telemetry":
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		anomaly := r.URL.Query().Get("anomaly") == "true"

		t := Telemetry{
			DeviceUID:   uid,
			Luminosite:  20 + rand.Float64()*10,
			Presence:    rand.Intn(2) == 1,
			Temperature: 25 + rand.Float64()*10,
			Humidite:    40 + rand.Float64()*20,
			Tension:     220 + rand.Float64()*5,
			Courant:     0.4 + rand.Float64()*0.1,
			Puissance:   float64(device.Puissance) * (float64(device.Intensite) / 100.0),
			Energie:     2.0 + rand.Float64(),
			Source:      "mock_lcu",
		}

		if anomaly {
			// Trigger one of the anomalies
			choice := rand.Intn(3)
			switch choice {
			case 0:
				t.Temperature = 85 + rand.Float64()*10
			case 1:
				t.Humidite = 95 + rand.Float64()*5
			case 2:
				t.Puissance = float64(device.Puissance) * 1.6
			}
			t.Source = "mock_lcu_anomaly"
		}

		json.NewEncoder(w).Encode(t)

	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}
