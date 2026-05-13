package repository

import (
	"database/sql"
	"time"

	"map-interactif/internal/models"
	"map-interactif/internal/utils"
)

// InsertCabinet inserts a new cabinet.
func InsertCabinet(db *sql.DB, cab *models.Cabinet) error {
	return db.QueryRow(`
		INSERT INTO cabinets (reference, name, zone, address, latitude, longitude,
			status, door_status, power_status, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id, created_at, updated_at`,
		cab.Reference, cab.Name, cab.Zone, cab.Address, cab.Latitude, cab.Longitude,
		utils.OrDefault(cab.Status, "unknown"),
		utils.OrDefault(cab.DoorStatus, "unknown"),
		utils.OrDefault(cab.PowerStatus, "normal"),
		cab.Notes,
	).Scan(&cab.ID, &cab.CreatedAt, &cab.UpdatedAt)
}

// ListCabinets returns all cabinets.
func ListCabinets(db *sql.DB) ([]models.Cabinet, error) {
	rows, err := db.Query(`
		SELECT id, reference, name, zone, address, latitude, longitude,
			status, door_status, power_status,
			voltage_l1, voltage_l2, voltage_l3,
			current_l1, current_l2, current_l3,
			leakage_current, energy_kwh, last_seen_at, notes,
			created_at, updated_at
		FROM cabinets ORDER BY zone, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.Cabinet
	for rows.Next() {
		if cab, err := scanCabinetRow(rows.Scan); err == nil {
			list = append(list, cab)
		}
	}
	return list, nil
}

// GetCabinetByID fetches a single cabinet by ID.
func GetCabinetByID(db *sql.DB, id int) (*models.Cabinet, error) {
	row := db.QueryRow(`
		SELECT id, reference, name, zone, address, latitude, longitude,
			status, door_status, power_status,
			voltage_l1, voltage_l2, voltage_l3,
			current_l1, current_l2, current_l3,
			leakage_current, energy_kwh, last_seen_at, notes,
			created_at, updated_at
		FROM cabinets WHERE id = $1`, id)
	cab, err := scanCabinetRow(row.Scan)
	if err != nil {
		return nil, err
	}
	return &cab, nil
}

// UpdateCabinetFields updates cabinet status fields.
func UpdateCabinetFields(db *sql.DB, id int, fields map[string]any) error {
	fields["updated_at"] = time.Now()
	_, err := db.Exec(`UPDATE cabinets SET status=$1, door_status=$2, power_status=$3, updated_at=$4 WHERE id=$5`,
		fields["status"], fields["door_status"], fields["power_status"], fields["updated_at"], id)
	return err
}

// ListCabinetCircuits returns all circuits for a given cabinet.
func ListCabinetCircuits(db *sql.DB, cabinetID int) ([]models.CabinetCircuit, error) {
	rows, err := db.Query(`
		SELECT id, cabinet_id, name, phase, circuit_number,
			status, contactor_status, breaker_status,
			measured_current, measured_voltage, measured_power,
			lamp_count, profile_id, last_fault_at, created_at, updated_at
		FROM cabinet_circuits WHERE cabinet_id = $1 ORDER BY circuit_number`, cabinetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.CabinetCircuit
	for rows.Next() {
		var cc models.CabinetCircuit
		var curr, volt, pwr sql.NullFloat64
		var lastFault sql.NullTime
		var profileID sql.NullInt64
		if err := rows.Scan(
			&cc.ID, &cc.CabinetID, &cc.Name, &cc.Phase, &cc.CircuitNumber,
			&cc.Status, &cc.ContactorStatus, &cc.BreakerStatus,
			&curr, &volt, &pwr,
			&cc.LampCount, &profileID, &lastFault,
			&cc.CreatedAt, &cc.UpdatedAt,
		); err == nil {
			if curr.Valid {
				cc.MeasuredCurrent = &curr.Float64
			}
			if volt.Valid {
				cc.MeasuredVoltage = &volt.Float64
			}
			if pwr.Valid {
				cc.MeasuredPower = &pwr.Float64
			}
			if lastFault.Valid {
				cc.LastFaultAt = &lastFault.Time
			}
			if profileID.Valid {
				v := int(profileID.Int64)
				cc.ProfileID = &v
			}
			list = append(list, cc)
		}
	}
	return list, nil
}

// InsertCabinetCircuit inserts a new circuit for a cabinet.
func InsertCabinetCircuit(db *sql.DB, cc *models.CabinetCircuit) error {
	return db.QueryRow(`
		INSERT INTO cabinet_circuits (cabinet_id, name, phase, circuit_number, status, lamp_count)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id, created_at, updated_at`,
		cc.CabinetID, cc.Name,
		utils.OrDefault(cc.Phase, "L1"),
		cc.CircuitNumber,
		utils.OrDefault(cc.Status, "active"),
		cc.LampCount,
	).Scan(&cc.ID, &cc.CreatedAt, &cc.UpdatedAt)
}

func scanCabinetRow(scan func(...any) error) (models.Cabinet, error) {
	var cab models.Cabinet
	var lat, lng sql.NullFloat64
	var v1, v2, v3, c1, c2, c3, leak sql.NullFloat64
	var lastSeen sql.NullTime
	err := scan(
		&cab.ID, &cab.Reference, &cab.Name, &cab.Zone, &cab.Address,
		&lat, &lng,
		&cab.Status, &cab.DoorStatus, &cab.PowerStatus,
		&v1, &v2, &v3, &c1, &c2, &c3,
		&leak, &cab.EnergyKwh, &lastSeen, &cab.Notes,
		&cab.CreatedAt, &cab.UpdatedAt,
	)
	if err != nil {
		return cab, err
	}
	nullF := func(n sql.NullFloat64) *float64 {
		if n.Valid {
			return &n.Float64
		}
		return nil
	}
	cab.Latitude = nullF(lat)
	cab.Longitude = nullF(lng)
	cab.VoltageL1 = nullF(v1)
	cab.VoltageL2 = nullF(v2)
	cab.VoltageL3 = nullF(v3)
	cab.CurrentL1 = nullF(c1)
	cab.CurrentL2 = nullF(c2)
	cab.CurrentL3 = nullF(c3)
	cab.LeakageCurrent = nullF(leak)
	if lastSeen.Valid {
		cab.LastSeenAt = &lastSeen.Time
	}
	return cab, nil
}
