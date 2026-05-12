package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
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

type zoneConfig struct {
	name string
	lat  float64
	lng  float64
}

var zones = []zoneConfig{
	{"Zone A (Medina)", 31.6295, -7.9811},
	{"Zone B (Gueliz)", 31.6340, -8.0100},
	{"Zone C (Hivernage)", 31.6205, -8.0050},
	{"Zone D (Palmeraie)", 31.6600, -7.9500},
}

var drivers = []string{"DALI", "0-10V", "PWM"}
var powers = []int{50, 70, 80, 90, 100, 120, 150}

func init() {
	count := 30
	if v := os.Getenv("LAMP_COUNT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			count = n
		}
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// How many devices per zone get no GPS (to test "missing location" flow)
	// We mark the last 3 devices of each zone as no-coords
	noCoordPerZone := 3

	// Track how many devices we've assigned per zone
	zoneCount := make([]int, len(zones))

	for i := 0; i < count; i++ {
		z := zones[i%len(zones)]
		zoneIdx := i % len(zones)
		zoneCount[zoneIdx]++

		driver := drivers[i%len(drivers)]
		power := powers[i%len(powers)]

		// Determine state: ~70% online, ~15% maintenance, ~15% offline
		var etat string
		r := rng.Intn(100)
		switch {
		case r < 70:
			etat = "online"
		case r < 85:
			etat = "maintenance"
		default:
			etat = "offline"
		}

		intensite := 0
		if etat == "online" {
			intensite = 50 + rng.Intn(41) // 50–90
		}

		// GPS: assign coords unless this device is one of the last noCoordPerZone in its zone
		// We determine "last N in zone" by checking the total devices this zone will have
		devicesInZone := count / len(zones)
		if zoneIdx < count%len(zones) {
			devicesInZone++
		}
		hasCoords := zoneCount[zoneIdx] <= devicesInZone-noCoordPerZone
		if devicesInZone <= noCoordPerZone {
			hasCoords = true // avoid all being nil when zone is tiny
		}

		var lat, lng *float64
		if hasCoords {
			latVal := z.lat + (rng.Float64()-0.5)*0.001
			lngVal := z.lng + (rng.Float64()-0.5)*0.001
			lat = &latVal
			lng = &lngVal
		}

		uid := fmt.Sprintf("LCU-TEST-001-LAMP-%03d", i+1)
		d := &Device{
			DeviceUID:   uid,
			Reference:   fmt.Sprintf("LP-%03d", i+1),
			NodeAddress: fmt.Sprintf("0x%02X", i+1),
			Latitude:    lat,
			Longitude:   lng,
			Zone:        z.name,
			TypeDriver:  driver,
			Protocole:   "ZigBee",
			Puissance:   power,
			Etat:        etat,
			Intensite:   intensite,
		}
		devices[uid] = d
	}

	log.Printf("LCU virtuelle: %d lampadaires générés dans %d zones", count, len(zones))
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/devices/count", handleDeviceCount)
	mux.HandleFunc("/api/devices", handleDevices)
	mux.HandleFunc("/api/devices/", handleDeviceAction)

	port := ":9091"
	fmt.Printf("LCU Virtuelle demarree sur http://localhost%s\n", port)
	log.Fatal(http.ListenAndServe(port, mux))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]string{
		"status":    "online",
		"reference": "LCU-TEST-001",
		"firmware":  "1.0.0",
		"protocol":  "HTTP",
		"time":      time.Now().Format(time.RFC3339),
	}
	json.NewEncoder(w).Encode(resp)
}

func handleDeviceCount(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	n := len(devices)
	mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"count": n})
}

func handleDevices(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	defer mu.RUnlock()

	var list []*Device
	for _, d := range devices {
		list = append(list, d)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func handleDeviceAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	// /api/devices/{uid}/{action}
	if len(parts) < 4 {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	uid := parts[3]
	// "count" is handled by its own route; guard here in case mux routes it here
	if uid == "count" {
		handleDeviceCount(w, r)
		return
	}

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
		if req.Intensity > 0 {
			device.Etat = "online"
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":     "applied",
			"device_uid": uid,
			"intensity":  req.Intensity,
			"source":     req.Source,
			"reason":     req.Reason,
		})

	case "telemetry":
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		anomaly := r.URL.Query().Get("anomaly") == "true"
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))

		mu.RLock()
		puissance := device.Puissance
		intensite := device.Intensite
		mu.RUnlock()

		t := Telemetry{
			DeviceUID:   uid,
			Luminosite:  20 + rng.Float64()*10,
			Presence:    rng.Intn(2) == 1,
			Temperature: 25 + rng.Float64()*10,
			Humidite:    40 + rng.Float64()*20,
			Tension:     220 + rng.Float64()*5,
			Courant:     0.4 + rng.Float64()*0.1,
			Puissance:   float64(puissance) * (float64(intensite) / 100.0),
			Energie:     2.0 + rng.Float64(),
			Source:      "mock_lcu",
		}

		if anomaly {
			switch rng.Intn(3) {
			case 0:
				t.Temperature = 85 + rng.Float64()*10
			case 1:
				t.Humidite = 95 + rng.Float64()*5
			case 2:
				t.Puissance = float64(puissance) * 1.6
			}
			t.Source = "mock_lcu_anomaly"
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(t)

	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}
