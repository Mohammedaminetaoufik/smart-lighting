package services

import (
	"context"
	"database/sql"
	"fmt"

	"map-interactif/internal/models"
	"map-interactif/internal/repository"
)

// SyncResult carries the outcome of a single LCU sync operation.
type SyncResult struct {
	DiscoveredCount int
	CreatedCount    int
	UpdatedCount    int
	FailedCount     int
	ControllerCount int
	Message         string
}

// SyncLCU discovers devices from the LCU, upserts lampadaires with embedded
// controller info, and logs the result. It owns the transaction lifecycle.
func SyncLCU(ctx context.Context, db *sql.DB, adapter LCUAdapter, lcuID int) (SyncResult, error) {
	lcu, err := repository.GetLCUByID(ctx, db, lcuID)
	if err != nil {
		return SyncResult{}, fmt.Errorf("LCU introuvable: %w", err)
	}

	devices, err := adapter.DiscoverDevices(ctx, lcu)
	if err != nil {
		log := models.LCUSyncLog{
			LCUID:   lcuID,
			Status:  "failed",
			Message: "echec decouverte : " + err.Error(),
		}
		repository.InsertLCUSyncLog(ctx, db, log)
		return SyncResult{}, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return SyncResult{}, fmt.Errorf("erreur transaction: %w", err)
	}
	defer tx.Rollback()

	var result SyncResult
	result.DiscoveredCount = len(devices)

	for _, d := range devices {
		var existingID int
		var existingCommStatus string
		queryErr := tx.QueryRowContext(ctx,
			"SELECT id, commissioning_status FROM lampadaires WHERE lcu_id = $1 AND device_uid = $2",
			lcuID, d.DeviceUID,
		).Scan(&existingID, &existingCommStatus)

		lamp := mapDeviceToLampadaire(d, lcuID, lcu.Latitude, lcu.Longitude, existingCommStatus)

		if queryErr == sql.ErrNoRows {
			if err := repository.InsertLampadaireTx(ctx, tx, lamp); err != nil {
				result.FailedCount++
			} else {
				result.CreatedCount++
			}
		} else if queryErr == nil {
			lamp.ID = existingID
			if err := repository.UpdateLampadaireTx(ctx, tx, lamp); err != nil {
				result.FailedCount++
			} else {
				result.UpdatedCount++
			}
		} else {
			result.FailedCount++
		}

		if d.ControllerType != "" || d.ControllerEmbedded {
			result.ControllerCount++
		}
	}

	syncLog := models.LCUSyncLog{
		LCUID:           lcuID,
		Status:          "success",
		Message:         fmt.Sprintf("Synchronisation terminée : %d découverts, %d créés, %d mis à jour, %d erreurs", result.DiscoveredCount, result.CreatedCount, result.UpdatedCount, result.FailedCount),
		DiscoveredCount: result.DiscoveredCount,
		CreatedCount:    result.CreatedCount,
		UpdatedCount:    result.UpdatedCount,
		FailedCount:     result.FailedCount,
	}
	repository.InsertLCUSyncLogTx(ctx, tx, syncLog)

	tx.ExecContext(ctx, "UPDATE lcus SET last_sync_at = NOW() WHERE id = $1", lcuID)

	if err := tx.Commit(); err != nil {
		return SyncResult{}, fmt.Errorf("erreur commit: %w", err)
	}

	result.Message = syncLog.Message
	return result, nil
}

// mapDeviceToLampadaire converts a discovered device DTO into a Lampadaire,
// applying location fallback from the LCU and commissioning status progression.
func mapDeviceToLampadaire(d models.LcuDeviceDTO, lcuID int, lcuLat, lcuLng *float64, existingCommStatus string) models.Lampadaire {
	locStatus := "confirmed"
	lat := d.Latitude
	lng := d.Longitude

	if lat == nil || lng == nil {
		locStatus = "estimated"
		lat = lcuLat
		lng = lcuLng
	}

	commStatus := "discovered"
	if existingCommStatus != "" {
		commStatus = existingCommStatus
	}
	if commStatus == "discovered" && lat != nil && lng != nil {
		commStatus = "located"
	}

	// Default to Zhaga Book 18 for all discovered devices
	ctrlType := d.ControllerType
	if ctrlType == "" {
		ctrlType = "Zhaga Book 18"
	}
	ctrlStatus := "ok"
	ctrlUID := d.DeviceUID // controller UID == device UID (embedded)

	return models.Lampadaire{
		Reference:           d.Reference,
		Latitude:            lat,
		Longitude:           lng,
		Zone:                d.Zone,
		TypeDriver:          d.TypeDriver,
		Protocole:           d.Protocole,
		Puissance:           d.Puissance,
		Etat:                d.Etat,
		Intensite:           d.Intensite,
		LCUID:               &lcuID,
		DeviceUID:           d.DeviceUID,
		NodeAddress:         d.NodeAddress,
		DiscoveredByLCU:     true,
		LocationStatus:      locStatus,
		CommissioningStatus: commStatus,

		ControllerUID:           ctrlUID,
		ControllerType:          ctrlType,
		ControllerStatus:        ctrlStatus,
		ControllerSignalQuality: d.ControllerSignalQuality,
		ControllerFirmware:      d.ControllerFirmware,
		ControllerEmbedded:      true,
		DimmingEnabled:          d.DimmingEnabled,
		MeteringEnabled:         d.MeteringEnabled,
		ArmoireReference:        d.ArmoireReference,
		CircuitReference:        d.CircuitReference,

		DriverBrand:          d.DriverBrand,
		DriverModel:          d.DriverModel,
		DriverProtocol:       d.DriverProtocol,
		NominalPowerW:        d.NominalPowerW,
		OutputCurrentMA:      d.OutputCurrentMA,
		OutputVoltageV:       d.OutputVoltageV,
		PowerFactor:          d.PowerFactor,
		SurgeProtection:      d.SurgeProtection,
		DimmingProtocol:      d.DimmingProtocol,
		D4ICompatible:        d.D4ICompatible,
		DriverTemperature:    d.DriverTemperature,
		LEDModuleTemperature: d.LEDModuleTemperature,
		EnergyKWh:            d.EnergyKWh,
		OperatingHours:       d.OperatingHours,
		FaultStatus:          d.FaultStatus,
	}
}
