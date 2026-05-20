package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"time"

	"map-interactif/internal/models"
)

// LCUAdapter defines the interface for communicating with LCUs.
type LCUAdapter interface {
	Health(ctx context.Context, lcu *models.LCU) error
	DiscoverDevices(ctx context.Context, lcu *models.LCU) ([]models.LcuDeviceDTO, error)
	ApplyDimming(ctx context.Context, lcu *models.LCU, deviceUID string, intensity int, reason string, source string) error
}

// NewLCUAdapter returns an LCU adapter based on the environment configuration.
func NewLCUAdapter() LCUAdapter {
	mode := os.Getenv("LCU_DISCOVERY_MODE")
	if mode == "http" {
		return &HTTPLCUAdapter{Timeout: 5 * time.Second}
	}
	return &MockLCUAdapter{}
}

// MockLCUAdapter is a simulation adapter for testing.
type MockLCUAdapter struct{}

func (a *MockLCUAdapter) Health(ctx context.Context, lcu *models.LCU) error {
	time.Sleep(200 * time.Millisecond)
	if lcu.IPAddress == "0.0.0.0" {
		return fmt.Errorf("LCU offline (simulated)")
	}
	return nil
}

func (a *MockLCUAdapter) DiscoverDevices(ctx context.Context, lcu *models.LCU) ([]models.LcuDeviceDTO, error) {
	time.Sleep(500 * time.Millisecond)

	driverBrands := []string{"Tridonic", "Helvar", "Osram", "Inventronics"}
	driverModels := []string{"LCA 50W 150V", "LCAI 50W 900mA", "OTi 50/220-240/1A4", "EUM050S070DK"}

	count := 5 + rand.Intn(4) // 5 to 8 devices
	devices := make([]models.LcuDeviceDTO, count)

	for i := 0; i < count; i++ {
		uid := fmt.Sprintf("%s-LAMP-%03d", lcu.Reference, i+1)
		ref := fmt.Sprintf("LP-%03d", i+1)
		node := fmt.Sprintf("0x%02X", i+1)

		puissance := 50 + rand.Intn(100)
		signalQuality := 60 + rand.Intn(40)
		armoireRef := fmt.Sprintf("ARM-%s", lcu.Reference)
		circuitRef := fmt.Sprintf("CIR-%02d", (i%2)+1)

		nomPower := puissance
		outCurrent := 350.0 + float64(rand.Intn(650))
		outVoltage := 48.0 + float64(rand.Intn(52))
		pf := 0.90 + rand.Float64()*0.09
		drvTemp := 35.0 + rand.Float64()*25.0
		ledTemp := 40.0 + rand.Float64()*30.0
		opHours := float64(rand.Intn(8760))
		faultOptions := []string{"none", "none", "none", "none", "overtemp"}

		d := models.LcuDeviceDTO{
			DeviceUID:        uid,
			Reference:        ref,
			NodeAddress:      node,
			Zone:             lcu.Zone,
			TypeDriver:       "DALI-2",
			Protocole:        "ZigBee",
			Puissance:        &puissance,
			Etat:             "online",
			Intensite:        70,
			DimmingEnabled:   true,
			MeteringEnabled:  true,
			ArmoireReference: armoireRef,
			CircuitReference: circuitRef,

			// All devices have Zhaga Book 18 embedded controller
			ControllerType:          "Zhaga Book 18",
			ControllerFirmware:      fmt.Sprintf("v3.%d.%d", rand.Intn(5), rand.Intn(10)),
			ControllerSignalQuality: &signalQuality,
			ControllerEmbedded:      true,

			// Driver technical data
			DriverBrand:          driverBrands[i%len(driverBrands)],
			DriverModel:          driverModels[i%len(driverModels)],
			DriverProtocol:       "DALI-2",
			NominalPowerW:        &nomPower,
			OutputCurrentMA:      &outCurrent,
			OutputVoltageV:       &outVoltage,
			PowerFactor:          &pf,
			SurgeProtection:      true,
			DimmingProtocol:      "DALI",
			D4ICompatible:        true,
			DriverTemperature:    &drvTemp,
			LEDModuleTemperature: &ledTemp,
			OperatingHours:       &opHours,
			FaultStatus:          faultOptions[rand.Intn(len(faultOptions))],
		}

		if i < count-2 {
			lat := 31.6251 + float64(i)*0.001
			lng := -7.9892 + float64(i)*0.001
			d.Latitude = &lat
			d.Longitude = &lng
		}

		devices[i] = d
	}

	return devices, nil
}

func (a *MockLCUAdapter) ApplyDimming(ctx context.Context, lcu *models.LCU, deviceUID string, intensity int, reason string, source string) error {
	time.Sleep(100 * time.Millisecond)
	return nil
}

// HTTPLCUAdapter communicates with a real LCU via HTTP.
type HTTPLCUAdapter struct {
	Timeout time.Duration
}

func (a *HTTPLCUAdapter) Health(ctx context.Context, lcu *models.LCU) error {
	url := fmt.Sprintf("http://%s:%d/api/health", lcu.IPAddress, lcu.Port)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	if lcu.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+lcu.AuthToken)
	}

	client := &http.Client{Timeout: a.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("LCU returned status %d", resp.StatusCode)
	}

	return nil
}

func (a *HTTPLCUAdapter) DiscoverDevices(ctx context.Context, lcu *models.LCU) ([]models.LcuDeviceDTO, error) {
	url := fmt.Sprintf("http://%s:%d/api/devices", lcu.IPAddress, lcu.Port)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	if lcu.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+lcu.AuthToken)
	}

	client := &http.Client{Timeout: a.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LCU returned status %d", resp.StatusCode)
	}

	var devices []models.LcuDeviceDTO
	if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
		return nil, err
	}

	return devices, nil
}

func (a *HTTPLCUAdapter) ApplyDimming(ctx context.Context, lcu *models.LCU, deviceUID string, intensity int, reason string, source string) error {
	url := fmt.Sprintf("http://%s:%d/api/devices/%s/dimming", lcu.IPAddress, lcu.Port, deviceUID)

	payload := map[string]interface{}{
		"intensity": intensity,
		"reason":    reason,
		"source":    source,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if lcu.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+lcu.AuthToken)
	}

	client := &http.Client{Timeout: a.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("LCU returned status %d", resp.StatusCode)
	}

	return nil
}
