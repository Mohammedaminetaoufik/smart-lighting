package main

import (
	"database/sql"
	"time"
)

func insertController(db *sql.DB, ctrl *Controller) error {
	return db.QueryRow(`
		INSERT INTO controllers (controller_uid, serial_number, type,
			lampadaire_id, basestation_id, cabinet_id, firmware_version,
			communication_status, signal_quality, metering_enabled, dimming_enabled, installation_status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id, created_at, updated_at`,
		ctrl.ControllerUID, ctrl.SerialNumber,
		orDefault(ctrl.Type, "Simulated"),
		nullableInt(ctrl.LampadaireID), nullableInt(ctrl.BasestationID), nullableInt(ctrl.CabinetID),
		ctrl.FirmwareVersion,
		orDefault(ctrl.CommunicationStatus, "ok"),
		ctrl.SignalQuality,
		ctrl.MeteringEnabled, ctrl.DimmingEnabled,
		orDefault(ctrl.InstallationStatus, "discovered"),
	).Scan(&ctrl.ID, &ctrl.CreatedAt, &ctrl.UpdatedAt)
}

func listControllers(db *sql.DB) ([]Controller, error) {
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
	var list []Controller
	for rows.Next() {
		if ctrl, err := scanControllerRow(rows.Scan); err == nil {
			list = append(list, ctrl)
		}
	}
	return list, nil
}

func listControllersByBasestation(db *sql.DB, basestationID int) ([]Controller, error) {
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
	var list []Controller
	for rows.Next() {
		if ctrl, err := scanControllerRow(rows.Scan); err == nil {
			list = append(list, ctrl)
		}
	}
	return list, nil
}

func getControllerByID(db *sql.DB, id int) (*Controller, error) {
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

func associateControllerToLampadaire(db *sql.DB, controllerID, lampadaireID int) error {
	now := time.Now()
	_, err := db.Exec(`
		UPDATE controllers SET lampadaire_id=$1, installation_status='associated', updated_at=$2
		WHERE id=$3`,
		lampadaireID, now, controllerID)
	return err
}

func scanControllerRow(scan func(...any) error) (Controller, error) {
	var ctrl Controller
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

func nullableInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}
