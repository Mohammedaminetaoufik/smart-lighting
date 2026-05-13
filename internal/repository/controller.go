package repository

import (
	"database/sql"
	"time"

	"map-interactif/internal/models"
	"map-interactif/internal/utils"
)

// InsertController inserts a new controller.
func InsertController(db *sql.DB, ctrl *models.Controller) error {
	return db.QueryRow(`
		INSERT INTO controllers (controller_uid, serial_number, type,
			lampadaire_id, basestation_id, cabinet_id, firmware_version,
			communication_status, signal_quality, metering_enabled, dimming_enabled, installation_status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id, created_at, updated_at`,
		ctrl.ControllerUID, ctrl.SerialNumber,
		utils.OrDefault(ctrl.Type, "Simulated"),
		NullableInt(ctrl.LampadaireID), NullableInt(ctrl.BasestationID), NullableInt(ctrl.CabinetID),
		ctrl.FirmwareVersion,
		utils.OrDefault(ctrl.CommunicationStatus, "ok"),
		ctrl.SignalQuality,
		ctrl.MeteringEnabled, ctrl.DimmingEnabled,
		utils.OrDefault(ctrl.InstallationStatus, "discovered"),
	).Scan(&ctrl.ID, &ctrl.CreatedAt, &ctrl.UpdatedAt)
}

// ListControllers returns all controllers.
func ListControllers(db *sql.DB) ([]models.Controller, error) {
	rows, err := db.Query(`
		SELECT id, controller_uid, serial_number, type,
			lampadaire_id, basestation_id, cabinet_id, firmware_version,
			communication_status, signal_quality, last_seen_at,
			metering_enabled, dimming_enabled, installation_status,
			created_at, updated_at
		FROM controllers ORDER BY controller_uid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.Controller
	for rows.Next() {
		if ctrl, err := scanControllerRow(rows.Scan); err == nil {
			list = append(list, ctrl)
		}
	}
	return list, nil
}

// ListControllersByBasestation returns all controllers for a given basestation.
func ListControllersByBasestation(db *sql.DB, basestationID int) ([]models.Controller, error) {
	rows, err := db.Query(`
		SELECT id, controller_uid, serial_number, type,
			lampadaire_id, basestation_id, cabinet_id, firmware_version,
			communication_status, signal_quality, last_seen_at,
			metering_enabled, dimming_enabled, installation_status,
			created_at, updated_at
		FROM controllers WHERE basestation_id = $1 ORDER BY controller_uid`, basestationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.Controller
	for rows.Next() {
		if ctrl, err := scanControllerRow(rows.Scan); err == nil {
			list = append(list, ctrl)
		}
	}
	return list, nil
}

// GetControllerByID fetches a single controller by ID.
func GetControllerByID(db *sql.DB, id int) (*models.Controller, error) {
	row := db.QueryRow(`
		SELECT id, controller_uid, serial_number, type,
			lampadaire_id, basestation_id, cabinet_id, firmware_version,
			communication_status, signal_quality, last_seen_at,
			metering_enabled, dimming_enabled, installation_status,
			created_at, updated_at
		FROM controllers WHERE id = $1`, id)
	ctrl, err := scanControllerRow(row.Scan)
	if err != nil {
		return nil, err
	}
	return &ctrl, nil
}

// AssociateControllerToLampadaire associates a controller with a lampadaire.
func AssociateControllerToLampadaire(db *sql.DB, controllerID, lampadaireID int) error {
	now := time.Now()
	_, err := db.Exec(`
		UPDATE controllers SET lampadaire_id=$1, installation_status='associated', updated_at=$2
		WHERE id=$3`,
		lampadaireID, now, controllerID)
	return err
}

func scanControllerRow(scan func(...any) error) (models.Controller, error) {
	var ctrl models.Controller
	var lampID, bsID, cabID sql.NullInt64
	var lastSeen sql.NullTime
	err := scan(
		&ctrl.ID, &ctrl.ControllerUID, &ctrl.SerialNumber, &ctrl.Type,
		&lampID, &bsID, &cabID,
		&ctrl.FirmwareVersion, &ctrl.CommunicationStatus, &ctrl.SignalQuality, &lastSeen,
		&ctrl.MeteringEnabled, &ctrl.DimmingEnabled, &ctrl.InstallationStatus,
		&ctrl.CreatedAt, &ctrl.UpdatedAt,
	)
	if err != nil {
		return ctrl, err
	}
	if lampID.Valid {
		v := int(lampID.Int64)
		ctrl.LampadaireID = &v
	}
	if bsID.Valid {
		v := int(bsID.Int64)
		ctrl.BasestationID = &v
	}
	if cabID.Valid {
		v := int(cabID.Int64)
		ctrl.CabinetID = &v
	}
	if lastSeen.Valid {
		ctrl.LastSeenAt = &lastSeen.Time
	}
	return ctrl, nil
}

// NullableInt returns nil for nil pointer, or the dereferenced int as interface{}.
func NullableInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}
