package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"time"
)

// LCUAdapter defines the interface for communicating with LCUs.
type LCUAdapter interface {
	Health(ctx context.Context, lcu *LCU) error
	DiscoverDevices(ctx context.Context, lcu *LCU) ([]LcuDeviceDTO, error)
	ApplyDimming(ctx context.Context, lcu *LCU, deviceUID string, intensity int) error
}

// NewLCUAdapter returns an LCU adapter based on the environment configuration.
func NewLCUAdapter() LCUAdapter {
	mode := os.Getenv("LCU_DISCOVERY_MODE")
	if mode == "http" {
		return &HTTPLCUAdapter{}
	}
	return &MockLCUAdapter{}
}

// MockLCUAdapter is a simulation adapter for testing.
type MockLCUAdapter struct{}

func (a *MockLCUAdapter) Health(ctx context.Context, lcu *LCU) error {
	// Simulate network delay
	time.Sleep(200 * time.Millisecond)
	if lcu.IPAddress == "0.0.0.0" {
		return fmt.Errorf("LCU offline (simulated)")
	}
	return nil
}

func (a *MockLCUAdapter) DiscoverDevices(ctx context.Context, lcu *LCU) ([]LcuDeviceDTO, error) {
	time.Sleep(500 * time.Millisecond)

	count := 5 + rand.Intn(4) // 5 to 8 devices
	devices := make([]LcuDeviceDTO, count)

	for i := 0; i < count; i++ {
		uid := fmt.Sprintf("%s-LAMP-%03d", lcu.Reference, i+1)
		ref := fmt.Sprintf("LP-%03d", i+1)
		node := fmt.Sprintf("0x%02X", i+1)

		d := LcuDeviceDTO{
			DeviceUID:   uid,
			Reference:   ref,
			NodeAddress: node,
			Zone:        lcu.Zone,
			TypeDriver:  "DALI",
			Protocole:   "ZigBee",
			Puissance:   ptrInt(100 + rand.Intn(50)),
			Etat:        "online",
			Intensite:   70,
		}

		// Some devices have no coordinates (to test location completion)
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

func (a *MockLCUAdapter) ApplyDimming(ctx context.Context, lcu *LCU, deviceUID string, intensity int) error {
	time.Sleep(100 * time.Millisecond)
	return nil
}

// HTTPLCUAdapter communicates with a real LCU via HTTP.
type HTTPLCUAdapter struct{}

func (a *HTTPLCUAdapter) Health(ctx context.Context, lcu *LCU) error {
	url := fmt.Sprintf("http://%s:%d/api/health", lcu.IPAddress, lcu.Port)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	if lcu.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+lcu.AuthToken)
	}

	client := &http.Client{Timeout: 3 * time.Second}
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

func (a *HTTPLCUAdapter) DiscoverDevices(ctx context.Context, lcu *LCU) ([]LcuDeviceDTO, error) {
	url := fmt.Sprintf("http://%s:%d/api/devices", lcu.IPAddress, lcu.Port)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	if lcu.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+lcu.AuthToken)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LCU returned status %d", resp.StatusCode)
	}

	var devices []LcuDeviceDTO
	if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
		return nil, err
	}

	return devices, nil
}

func (a *HTTPLCUAdapter) ApplyDimming(ctx context.Context, lcu *LCU, deviceUID string, intensity int) error {
	url := fmt.Sprintf("http://%s:%d/api/devices/%s/dimming", lcu.IPAddress, lcu.Port, deviceUID)

	body := map[string]interface{}{
		"intensity": intensity,
		"reason":    "Command from platform",
		"source":    "admin",
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if lcu.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+lcu.AuthToken)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("LCU returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func ptrInt(v int) *int {
	return &v
}
