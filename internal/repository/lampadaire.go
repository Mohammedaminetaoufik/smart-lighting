package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"map-interactif/internal/models"
	"map-interactif/internal/utils"
)

// GetLampadaireByID fetches a single lampadaire by its ID.
func GetLampadaireByID(ctx context.Context, db *sql.DB, id int) (*models.Lampadaire, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, reference, latitude, longitude, zone, type_driver, protocole,
			puissance, etat, intensite, date_installation,
			last_seen_at, last_command_at, address, quartier, lcu_reference, driver_reference, notes,
			lcu_id, device_uid, node_address, discovered_by_lcu, location_status, commissioning_status,
			controller_uid, controller_type, controller_status, controller_signal_quality,
			controller_firmware, controller_last_seen_at, controller_embedded,
			dimming_enabled, metering_enabled, armoire_reference, circuit_reference,
			cabinet_id, circuit_id,
			driver_brand, driver_model, driver_protocol, nominal_power_w,
			output_current_ma, output_voltage_v, power_factor, surge_protection,
			dimming_protocol, d4i_compatible, driver_temperature, led_module_temperature,
			energy_kwh, operating_hours, fault_status,
			commissioning_notes, test_comm_status, test_dimming_status
		FROM lampadaires
		WHERE id = $1 AND archived_at IS NULL
	`, id)

	return scanLampadaireSingle(row)
}

func scanLampadaireSingle(row *sql.Row) (*models.Lampadaire, error) {
	var item models.Lampadaire
	var zone, typeDriver, protocole sql.NullString
	var puissance, lcuID sql.NullInt64
	var lat, lng sql.NullFloat64
	var dateInstallation, lastSeenAt, lastCommandAt sql.NullTime
	var address, quartier, lcuReference, driverReference, notes, deviceUID, nodeAddress sql.NullString

	var ctrlUID, ctrlType, ctrlStatus, ctrlFirmware sql.NullString
	var ctrlSignal, cabID, circID sql.NullInt64
	var ctrlLastSeen sql.NullTime
	var ctrlEmbedded, dimmingEnabled, meteringEnabled sql.NullBool
	var armoireRef, circuitRef sql.NullString

	var drvBrand, drvModel, drvProtocol, dimmingProto, faultStatus sql.NullString
	var nomPowerW sql.NullInt64
	var outCurrentMA, outVoltageV, powerFactor, drvTemp, ledTemp, energyKWh, opHours sql.NullFloat64
	var surgeProt, d4iCompat sql.NullBool
	var commNotes, testCommStatus, testDimmingStatus sql.NullString

	err := row.Scan(
		&item.ID, &item.Reference, &lat, &lng,
		&zone, &typeDriver, &protocole, &puissance,
		&item.Etat, &item.Intensite, &dateInstallation,
		&lastSeenAt, &lastCommandAt, &address, &quartier,
		&lcuReference, &driverReference, &notes,
		&lcuID, &deviceUID, &nodeAddress, &item.DiscoveredByLCU, &item.LocationStatus, &item.CommissioningStatus,
		&ctrlUID, &ctrlType, &ctrlStatus, &ctrlSignal,
		&ctrlFirmware, &ctrlLastSeen, &ctrlEmbedded,
		&dimmingEnabled, &meteringEnabled, &armoireRef, &circuitRef,
		&cabID, &circID,
		&drvBrand, &drvModel, &drvProtocol, &nomPowerW,
		&outCurrentMA, &outVoltageV, &powerFactor, &surgeProt,
		&dimmingProto, &d4iCompat, &drvTemp, &ledTemp,
		&energyKWh, &opHours, &faultStatus,
		&commNotes, &testCommStatus, &testDimmingStatus,
	)
	if err != nil {
		return nil, err
	}

	populateLampadaireFields(&item, lat, lng, zone, typeDriver, protocole, puissance, lcuID, dateInstallation, lastSeenAt, lastCommandAt, address, quartier, lcuReference, driverReference, notes, deviceUID, nodeAddress)
	populateControllerFields(&item, ctrlUID, ctrlType, ctrlStatus, ctrlFirmware, ctrlSignal, cabID, circID, ctrlLastSeen, ctrlEmbedded, dimmingEnabled, meteringEnabled, armoireRef, circuitRef)
	populateDriverFields(&item, drvBrand, drvModel, drvProtocol, dimmingProto, faultStatus, nomPowerW, outCurrentMA, outVoltageV, powerFactor, drvTemp, ledTemp, energyKWh, opHours, surgeProt, d4iCompat)
	if commNotes.Valid { item.CommissioningNotes = commNotes.String }
	if testCommStatus.Valid { item.TestCommStatus = testCommStatus.String }
	if testDimmingStatus.Valid { item.TestDimmingStatus = testDimmingStatus.String }
	return &item, nil
}

func populateLampadaireFields(item *models.Lampadaire, lat, lng sql.NullFloat64, zone, typeDriver, protocole sql.NullString, puissance, lcuID sql.NullInt64, dateInstallation, lastSeenAt, lastCommandAt sql.NullTime, address, quartier, lcuReference, driverReference, notes, deviceUID, nodeAddress sql.NullString) {
	if lat.Valid {
		item.Latitude = &lat.Float64
	}
	if lng.Valid {
		item.Longitude = &lng.Float64
	}
	if zone.Valid {
		item.Zone = zone.String
	}
	if typeDriver.Valid {
		item.TypeDriver = typeDriver.String
	}
	if protocole.Valid {
		item.Protocole = protocole.String
	}
	if puissance.Valid {
		v := int(puissance.Int64)
		item.Puissance = &v
	}
	if dateInstallation.Valid {
		f := dateInstallation.Time.Format("2006-01-02")
		item.DateInstallation = &f
	}
	if lastSeenAt.Valid {
		f := lastSeenAt.Time.Format("2006-01-02T15:04:05Z")
		item.LastSeenAt = &f
	}
	if lastCommandAt.Valid {
		f := lastCommandAt.Time.Format("2006-01-02T15:04:05Z")
		item.LastCommandAt = &f
	}
	if address.Valid {
		item.Address = address.String
	}
	if quartier.Valid {
		item.Quartier = quartier.String
	}
	if lcuReference.Valid {
		item.LCUReference = lcuReference.String
	}
	if driverReference.Valid {
		item.DriverReference = driverReference.String
	}
	if notes.Valid {
		item.Notes = notes.String
	}
	if lcuID.Valid {
		v := int(lcuID.Int64)
		item.LCUID = &v
	}
	if deviceUID.Valid {
		item.DeviceUID = deviceUID.String
	}
	if nodeAddress.Valid {
		item.NodeAddress = nodeAddress.String
	}
}

func populateControllerFields(item *models.Lampadaire,
	ctrlUID, ctrlType, ctrlStatus, ctrlFirmware sql.NullString,
	ctrlSignal, cabID, circID sql.NullInt64,
	ctrlLastSeen sql.NullTime,
	ctrlEmbedded, dimmingEnabled, meteringEnabled sql.NullBool,
	armoireRef, circuitRef sql.NullString,
) {
	if ctrlUID.Valid {
		item.ControllerUID = ctrlUID.String
	}
	if ctrlType.Valid {
		item.ControllerType = ctrlType.String
	}
	if ctrlStatus.Valid {
		item.ControllerStatus = ctrlStatus.String
	}
	if ctrlSignal.Valid {
		v := int(ctrlSignal.Int64)
		item.ControllerSignalQuality = &v
	}
	if ctrlFirmware.Valid {
		item.ControllerFirmware = ctrlFirmware.String
	}
	if ctrlLastSeen.Valid {
		f := ctrlLastSeen.Time.Format("2006-01-02T15:04:05Z")
		item.ControllerLastSeenAt = &f
	}
	if ctrlEmbedded.Valid {
		item.ControllerEmbedded = ctrlEmbedded.Bool
	}
	if dimmingEnabled.Valid {
		item.DimmingEnabled = dimmingEnabled.Bool
	}
	if meteringEnabled.Valid {
		item.MeteringEnabled = meteringEnabled.Bool
	}
	if armoireRef.Valid {
		item.ArmoireReference = armoireRef.String
	}
	if circuitRef.Valid {
		item.CircuitReference = circuitRef.String
	}
	if cabID.Valid {
		v := int(cabID.Int64)
		item.CabinetID = &v
	}
	if circID.Valid {
		v := int(circID.Int64)
		item.CircuitID = &v
	}
}

func populateDriverFields(item *models.Lampadaire,
	drvBrand, drvModel, drvProtocol, dimmingProto, faultStatus sql.NullString,
	nomPowerW sql.NullInt64,
	outCurrentMA, outVoltageV, powerFactor, drvTemp, ledTemp, energyKWh, opHours sql.NullFloat64,
	surgeProt, d4iCompat sql.NullBool,
) {
	if drvBrand.Valid {
		item.DriverBrand = drvBrand.String
	}
	if drvModel.Valid {
		item.DriverModel = drvModel.String
	}
	if drvProtocol.Valid {
		item.DriverProtocol = drvProtocol.String
	}
	if dimmingProto.Valid {
		item.DimmingProtocol = dimmingProto.String
	}
	if faultStatus.Valid {
		item.FaultStatus = faultStatus.String
	}
	if nomPowerW.Valid {
		v := int(nomPowerW.Int64)
		item.NominalPowerW = &v
	}
	if outCurrentMA.Valid {
		item.OutputCurrentMA = &outCurrentMA.Float64
	}
	if outVoltageV.Valid {
		item.OutputVoltageV = &outVoltageV.Float64
	}
	if powerFactor.Valid {
		item.PowerFactor = &powerFactor.Float64
	}
	if drvTemp.Valid {
		item.DriverTemperature = &drvTemp.Float64
	}
	if ledTemp.Valid {
		item.LEDModuleTemperature = &ledTemp.Float64
	}
	if energyKWh.Valid {
		item.EnergyKWh = &energyKWh.Float64
	}
	if opHours.Valid {
		item.OperatingHours = &opHours.Float64
	}
	if surgeProt.Valid {
		item.SurgeProtection = surgeProt.Bool
	}
	if d4iCompat.Valid {
		item.D4ICompatible = d4iCompat.Bool
	}
}

// buildLampadaireFilters builds the shared WHERE clause for list/count queries.
func buildLampadaireFilters(search map[string]string) ([]string, []interface{}) {
	where := []string{"archived_at IS NULL"}
	if search["archived"] == "1" {
		where = []string{"archived_at IS NOT NULL"}
	}
	args := []interface{}{}
	argID := 1

	if etat := search["etat"]; etat != "" {
		where = append(where, fmt.Sprintf("etat = $%d", argID))
		args = append(args, etat)
		argID++
	}
	if zone := search["zone"]; zone != "" {
		where = append(where, fmt.Sprintf("zone = $%d", argID))
		args = append(args, zone)
		argID++
	}
	if driver := search["driver"]; driver != "" {
		where = append(where, fmt.Sprintf("type_driver = $%d", argID))
		args = append(args, driver)
		argID++
	}
	if comm := search["commissioning"]; comm != "" {
		where = append(where, fmt.Sprintf("commissioning_status = $%d", argID))
		args = append(args, comm)
		argID++
	}
	if q := search["q"]; q != "" {
		where = append(where, fmt.Sprintf("(reference ILIKE $%d OR address ILIKE $%d OR quartier ILIKE $%d)", argID, argID, argID))
		args = append(args, "%"+q+"%")
		argID++
	}
	return where, args
}

// CountLampadaires returns the total count matching the same filters as ListLampadaires.
func CountLampadaires(ctx context.Context, db *sql.DB, search map[string]string) (int, error) {
	where, args := buildLampadaireFilters(search)
	var total int
	err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM lampadaires WHERE "+strings.Join(where, " AND "),
		args...,
	).Scan(&total)
	return total, err
}

// ListLampadaires returns non-archived lampadaires matching optional filters.
// Pagination: if limit > 0, LIMIT/OFFSET are applied (use with CountLampadaires).
func ListLampadaires(ctx context.Context, db *sql.DB, search map[string]string, limit, offset int) ([]models.Lampadaire, error) {
	where, args := buildLampadaireFilters(search)

	query := `
		SELECT l.id, l.reference, l.latitude, l.longitude, l.zone, l.type_driver, l.protocole,
			l.puissance, l.etat, l.intensite, l.date_installation,
			l.last_seen_at, l.last_command_at, l.address, l.quartier, l.lcu_reference, l.driver_reference, l.notes,
			l.lcu_id, l.device_uid, l.node_address, l.discovered_by_lcu, l.location_status, l.commissioning_status,
			l.controller_uid, l.controller_type, l.controller_status, l.controller_signal_quality,
			l.controller_firmware, l.controller_last_seen_at, l.controller_embedded,
			l.dimming_enabled, l.metering_enabled, l.armoire_reference, l.circuit_reference,
			l.cabinet_id, l.circuit_id,
			l.driver_brand, l.driver_model, l.driver_protocol, l.nominal_power_w,
			l.output_current_ma, l.output_voltage_v, l.power_factor, l.surge_protection,
			l.dimming_protocol, l.d4i_compatible, l.driver_temperature, l.led_module_temperature,
			l.energy_kwh, l.operating_hours, l.fault_status,
			l.commissioning_notes, l.test_comm_status, l.test_dimming_status,
			EXISTS(SELECT 1 FROM alerts a WHERE a.lampadaire_id = l.id AND a.status = 'open' AND a.severity = 'critical') as has_alert
		FROM lampadaires l
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY l.id
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
		args = append(args, limit, offset)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.Lampadaire
	for rows.Next() {
		var item models.Lampadaire
		var zone, typeDriver, protocole sql.NullString
		var puissance, lcuID sql.NullInt64
		var lat, lng sql.NullFloat64
		var dateInstallation, lastSeenAt, lastCommandAt sql.NullTime
		var address, quartier, lcuReference, driverReference, notes, deviceUID, nodeAddress sql.NullString

		var ctrlUID, ctrlType, ctrlStatus, ctrlFirmware sql.NullString
		var ctrlSignal, cabID, circID sql.NullInt64
		var ctrlLastSeen sql.NullTime
		var ctrlEmbedded, dimmingEnabled, meteringEnabled sql.NullBool
		var armoireRef, circuitRef sql.NullString

		var drvBrand, drvModel, drvProtocol, dimmingProto, faultStatus sql.NullString
		var nomPowerW sql.NullInt64
		var outCurrentMA, outVoltageV, powerFactor, drvTemp, ledTemp, energyKWh, opHours sql.NullFloat64
		var surgeProt, d4iCompat sql.NullBool
		var commNotes, testCommSt, testDimSt sql.NullString

		if err := rows.Scan(
			&item.ID, &item.Reference, &lat, &lng,
			&zone, &typeDriver, &protocole, &puissance,
			&item.Etat, &item.Intensite, &dateInstallation,
			&lastSeenAt, &lastCommandAt, &address, &quartier,
			&lcuReference, &driverReference, &notes,
			&lcuID, &deviceUID, &nodeAddress, &item.DiscoveredByLCU, &item.LocationStatus, &item.CommissioningStatus,
			&ctrlUID, &ctrlType, &ctrlStatus, &ctrlSignal,
			&ctrlFirmware, &ctrlLastSeen, &ctrlEmbedded,
			&dimmingEnabled, &meteringEnabled, &armoireRef, &circuitRef,
			&cabID, &circID,
			&drvBrand, &drvModel, &drvProtocol, &nomPowerW,
			&outCurrentMA, &outVoltageV, &powerFactor, &surgeProt,
			&dimmingProto, &d4iCompat, &drvTemp, &ledTemp,
			&energyKWh, &opHours, &faultStatus,
			&commNotes, &testCommSt, &testDimSt,
			&item.HasCriticalAlert,
		); err != nil {
			return nil, err
		}

		populateLampadaireFields(&item, lat, lng, zone, typeDriver, protocole, puissance, lcuID, dateInstallation, lastSeenAt, lastCommandAt, address, quartier, lcuReference, driverReference, notes, deviceUID, nodeAddress)
		populateControllerFields(&item, ctrlUID, ctrlType, ctrlStatus, ctrlFirmware, ctrlSignal, cabID, circID, ctrlLastSeen, ctrlEmbedded, dimmingEnabled, meteringEnabled, armoireRef, circuitRef)
		populateDriverFields(&item, drvBrand, drvModel, drvProtocol, dimmingProto, faultStatus, nomPowerW, outCurrentMA, outVoltageV, powerFactor, drvTemp, ledTemp, energyKWh, opHours, surgeProt, d4iCompat)
		if commNotes.Valid { item.CommissioningNotes = commNotes.String }
		if testCommSt.Valid { item.TestCommStatus = testCommSt.String }
		if testDimSt.Valid { item.TestDimmingStatus = testDimSt.String }
		result = append(result, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// InsertLampadaire inserts a new lampadaire.
func InsertLampadaire(ctx context.Context, db *sql.DB, l models.Lampadaire) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO lampadaires (
			reference, latitude, longitude, zone, type_driver, protocole, puissance,
			etat, intensite, date_installation, address, quartier, lcu_reference,
			driver_reference, notes, lcu_id, device_uid, node_address, discovered_by_lcu,
			location_status, commissioning_status,
			controller_uid, controller_type, controller_status, controller_signal_quality,
			controller_firmware, controller_embedded, dimming_enabled, metering_enabled,
			armoire_reference, circuit_reference, cabinet_id, circuit_id,
			driver_brand, driver_model, driver_protocol, nominal_power_w,
			output_current_ma, output_voltage_v, power_factor, surge_protection,
			dimming_protocol, d4i_compatible, driver_temperature, led_module_temperature,
			energy_kwh, operating_hours, fault_status
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,
			$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,
			$34,$35,$36,$37,$38,$39,$40,$41,$42,$43,$44,$45,$46,$47,$48
		)
	`,
		l.Reference, l.Latitude, l.Longitude,
		utils.NullString(l.Zone), utils.NullString(l.TypeDriver), utils.NullString(l.Protocole),
		l.Puissance, l.Etat, l.Intensite, l.DateInstallation,
		utils.NullString(l.Address), utils.NullString(l.Quartier), utils.NullString(l.LCUReference),
		utils.NullString(l.DriverReference), utils.NullString(l.Notes),
		l.LCUID, utils.NullString(l.DeviceUID), utils.NullString(l.NodeAddress),
		l.DiscoveredByLCU, l.LocationStatus, l.CommissioningStatus,
		utils.NullString(l.ControllerUID), utils.NullString(l.ControllerType),
		utils.NullString(l.ControllerStatus), l.ControllerSignalQuality,
		utils.NullString(l.ControllerFirmware), l.ControllerEmbedded,
		l.DimmingEnabled, l.MeteringEnabled,
		utils.NullString(l.ArmoireReference), utils.NullString(l.CircuitReference),
		l.CabinetID, l.CircuitID,
		utils.NullString(l.DriverBrand), utils.NullString(l.DriverModel),
		utils.NullString(l.DriverProtocol), l.NominalPowerW,
		l.OutputCurrentMA, l.OutputVoltageV, l.PowerFactor, l.SurgeProtection,
		utils.NullString(l.DimmingProtocol), l.D4ICompatible,
		l.DriverTemperature, l.LEDModuleTemperature,
		l.EnergyKWh, l.OperatingHours, utils.NullString(l.FaultStatus),
	)
	return err
}

// InsertLampadaireTx inserts a new lampadaire within a transaction.
func InsertLampadaireTx(ctx context.Context, tx *sql.Tx, l models.Lampadaire) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO lampadaires (
			reference, latitude, longitude, zone, type_driver, protocole, puissance,
			etat, intensite, date_installation, address, quartier, lcu_reference,
			driver_reference, notes, lcu_id, device_uid, node_address, discovered_by_lcu,
			location_status, commissioning_status,
			controller_uid, controller_type, controller_status, controller_signal_quality,
			controller_firmware, controller_embedded, dimming_enabled, metering_enabled,
			armoire_reference, circuit_reference, cabinet_id, circuit_id,
			driver_brand, driver_model, driver_protocol, nominal_power_w,
			output_current_ma, output_voltage_v, power_factor, surge_protection,
			dimming_protocol, d4i_compatible, driver_temperature, led_module_temperature,
			energy_kwh, operating_hours, fault_status
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,
			$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,
			$34,$35,$36,$37,$38,$39,$40,$41,$42,$43,$44,$45,$46,$47,$48
		)
	`,
		l.Reference, l.Latitude, l.Longitude,
		utils.NullString(l.Zone), utils.NullString(l.TypeDriver), utils.NullString(l.Protocole),
		l.Puissance, l.Etat, l.Intensite, l.DateInstallation,
		utils.NullString(l.Address), utils.NullString(l.Quartier), utils.NullString(l.LCUReference),
		utils.NullString(l.DriverReference), utils.NullString(l.Notes),
		l.LCUID, utils.NullString(l.DeviceUID), utils.NullString(l.NodeAddress),
		l.DiscoveredByLCU, l.LocationStatus, l.CommissioningStatus,
		utils.NullString(l.ControllerUID), utils.NullString(l.ControllerType),
		utils.NullString(l.ControllerStatus), l.ControllerSignalQuality,
		utils.NullString(l.ControllerFirmware), l.ControllerEmbedded,
		l.DimmingEnabled, l.MeteringEnabled,
		utils.NullString(l.ArmoireReference), utils.NullString(l.CircuitReference),
		l.CabinetID, l.CircuitID,
		utils.NullString(l.DriverBrand), utils.NullString(l.DriverModel),
		utils.NullString(l.DriverProtocol), l.NominalPowerW,
		l.OutputCurrentMA, l.OutputVoltageV, l.PowerFactor, l.SurgeProtection,
		utils.NullString(l.DimmingProtocol), l.D4ICompatible,
		l.DriverTemperature, l.LEDModuleTemperature,
		l.EnergyKWh, l.OperatingHours, utils.NullString(l.FaultStatus),
	)
	return err
}

// UpdateLampadaire updates the core fields of a lampadaire (used by the HTML form).
// Does NOT touch controller columns to avoid overwriting LCU-synced data.
func UpdateLampadaire(ctx context.Context, db *sql.DB, l models.Lampadaire) error {
	_, err := db.ExecContext(ctx, `
		UPDATE lampadaires
		SET reference = $1,
			latitude = $2,
			longitude = $3,
			zone = $4,
			type_driver = $5,
			protocole = $6,
			puissance = $7,
			etat = $8,
			intensite = $9,
			date_installation = $10,
			address = $11,
			quartier = $12,
			lcu_reference = $13,
			driver_reference = $14,
			notes = $15,
			lcu_id = $16,
			device_uid = $17,
			node_address = $18,
			discovered_by_lcu = $19,
			location_status = $20,
			commissioning_status = $21,
			updated_at = NOW()
		WHERE id = $22
	`,
		l.Reference, l.Latitude, l.Longitude,
		utils.NullString(l.Zone), utils.NullString(l.TypeDriver), utils.NullString(l.Protocole),
		l.Puissance, l.Etat, l.Intensite, l.DateInstallation,
		utils.NullString(l.Address), utils.NullString(l.Quartier), utils.NullString(l.LCUReference),
		utils.NullString(l.DriverReference), utils.NullString(l.Notes),
		l.LCUID, utils.NullString(l.DeviceUID), utils.NullString(l.NodeAddress), l.DiscoveredByLCU, l.LocationStatus, l.CommissioningStatus,
		l.ID,
	)
	return err
}

// UpdateLampadaireTx updates all fields of a lampadaire within a transaction,
// including controller and driver fields. Used exclusively by the LCU sync service.
func UpdateLampadaireTx(ctx context.Context, tx *sql.Tx, l models.Lampadaire) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE lampadaires
		SET reference = $1,
			latitude = $2,
			longitude = $3,
			zone = $4,
			type_driver = $5,
			protocole = $6,
			puissance = $7,
			etat = $8,
			intensite = $9,
			date_installation = $10,
			address = $11,
			quartier = $12,
			lcu_reference = $13,
			driver_reference = $14,
			notes = $15,
			lcu_id = $16,
			device_uid = $17,
			node_address = $18,
			discovered_by_lcu = $19,
			location_status = $20,
			commissioning_status = $21,
			controller_uid = $22,
			controller_type = $23,
			controller_status = $24,
			controller_signal_quality = $25,
			controller_firmware = $26,
			controller_embedded = $27,
			dimming_enabled = $28,
			metering_enabled = $29,
			armoire_reference = $30,
			circuit_reference = $31,
			cabinet_id = $32,
			circuit_id = $33,
			driver_brand = $34,
			driver_model = $35,
			driver_protocol = $36,
			nominal_power_w = $37,
			output_current_ma = $38,
			output_voltage_v = $39,
			power_factor = $40,
			surge_protection = $41,
			dimming_protocol = $42,
			d4i_compatible = $43,
			driver_temperature = $44,
			led_module_temperature = $45,
			energy_kwh = $46,
			operating_hours = $47,
			fault_status = $48,
			updated_at = NOW()
		WHERE id = $49
	`,
		l.Reference, l.Latitude, l.Longitude,
		utils.NullString(l.Zone), utils.NullString(l.TypeDriver), utils.NullString(l.Protocole),
		l.Puissance, l.Etat, l.Intensite, l.DateInstallation,
		utils.NullString(l.Address), utils.NullString(l.Quartier), utils.NullString(l.LCUReference),
		utils.NullString(l.DriverReference), utils.NullString(l.Notes),
		l.LCUID, utils.NullString(l.DeviceUID), utils.NullString(l.NodeAddress), l.DiscoveredByLCU, l.LocationStatus, l.CommissioningStatus,
		utils.NullString(l.ControllerUID), utils.NullString(l.ControllerType),
		utils.NullString(l.ControllerStatus), l.ControllerSignalQuality,
		utils.NullString(l.ControllerFirmware), l.ControllerEmbedded,
		l.DimmingEnabled, l.MeteringEnabled,
		utils.NullString(l.ArmoireReference), utils.NullString(l.CircuitReference),
		l.CabinetID, l.CircuitID,
		utils.NullString(l.DriverBrand), utils.NullString(l.DriverModel),
		utils.NullString(l.DriverProtocol), l.NominalPowerW,
		l.OutputCurrentMA, l.OutputVoltageV, l.PowerFactor, l.SurgeProtection,
		utils.NullString(l.DimmingProtocol), l.D4ICompatible,
		l.DriverTemperature, l.LEDModuleTemperature,
		l.EnergyKWh, l.OperatingHours, utils.NullString(l.FaultStatus),
		l.ID,
	)
	return err
}

// ListLampadairesMissingLocation returns lampadaires with missing or no coordinates.
func ListLampadairesMissingLocation(ctx context.Context, db *sql.DB) ([]models.Lampadaire, error) {
	query := `
		SELECT l.id, l.reference, l.latitude, l.longitude, l.zone, l.type_driver, l.protocole,
			l.puissance, l.etat, l.intensite, l.date_installation,
			l.last_seen_at, l.last_command_at, l.address, l.quartier, l.lcu_reference, l.driver_reference, l.notes,
			l.lcu_id, l.device_uid, l.node_address, l.discovered_by_lcu, l.location_status, l.commissioning_status,
			l.controller_uid, l.controller_type, l.controller_status, l.controller_signal_quality,
			l.controller_firmware, l.controller_last_seen_at, l.controller_embedded,
			l.dimming_enabled, l.metering_enabled, l.armoire_reference, l.circuit_reference,
			l.cabinet_id, l.circuit_id,
			l.driver_brand, l.driver_model, l.driver_protocol, l.nominal_power_w,
			l.output_current_ma, l.output_voltage_v, l.power_factor, l.surge_protection,
			l.dimming_protocol, l.d4i_compatible, l.driver_temperature, l.led_module_temperature,
			l.energy_kwh, l.operating_hours, l.fault_status,
			EXISTS(SELECT 1 FROM alerts a WHERE a.lampadaire_id = l.id AND a.status = 'open' AND a.severity = 'critical') as has_alert
		FROM lampadaires l
		WHERE (l.location_status = 'missing' OR l.latitude = 0 AND l.longitude = 0) AND l.archived_at IS NULL
		ORDER BY l.id
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.Lampadaire
	for rows.Next() {
		var item models.Lampadaire
		var zone, typeDriver, protocole sql.NullString
		var puissance, lid sql.NullInt64
		var lat, lng sql.NullFloat64
		var dateInstallation, lastSeenAt, lastCommandAt sql.NullTime
		var address, quartier, lcuReference, driverReference, notes, deviceUID, nodeAddress sql.NullString

		var ctrlUID, ctrlType, ctrlStatus, ctrlFirmware sql.NullString
		var ctrlSignal, cabID, circID sql.NullInt64
		var ctrlLastSeen sql.NullTime
		var ctrlEmbedded, dimmingEnabled, meteringEnabled sql.NullBool
		var armoireRef, circuitRef sql.NullString

		var drvBrand, drvModel, drvProtocol, dimmingProto, faultStatus sql.NullString
		var nomPowerW sql.NullInt64
		var outCurrentMA, outVoltageV, powerFactor, drvTemp, ledTemp, energyKWh, opHours sql.NullFloat64
		var surgeProt, d4iCompat sql.NullBool

		if err := rows.Scan(
			&item.ID, &item.Reference, &lat, &lng,
			&zone, &typeDriver, &protocole, &puissance,
			&item.Etat, &item.Intensite, &dateInstallation,
			&lastSeenAt, &lastCommandAt, &address, &quartier,
			&lcuReference, &driverReference, &notes,
			&lid, &deviceUID, &nodeAddress, &item.DiscoveredByLCU, &item.LocationStatus, &item.CommissioningStatus,
			&ctrlUID, &ctrlType, &ctrlStatus, &ctrlSignal,
			&ctrlFirmware, &ctrlLastSeen, &ctrlEmbedded,
			&dimmingEnabled, &meteringEnabled, &armoireRef, &circuitRef,
			&cabID, &circID,
			&drvBrand, &drvModel, &drvProtocol, &nomPowerW,
			&outCurrentMA, &outVoltageV, &powerFactor, &surgeProt,
			&dimmingProto, &d4iCompat, &drvTemp, &ledTemp,
			&energyKWh, &opHours, &faultStatus,
			&item.HasCriticalAlert,
		); err != nil {
			return nil, err
		}

		populateLampadaireFields(&item, lat, lng, zone, typeDriver, protocole, puissance, lid, dateInstallation, lastSeenAt, lastCommandAt, address, quartier, lcuReference, driverReference, notes, deviceUID, nodeAddress)
		populateControllerFields(&item, ctrlUID, ctrlType, ctrlStatus, ctrlFirmware, ctrlSignal, cabID, circID, ctrlLastSeen, ctrlEmbedded, dimmingEnabled, meteringEnabled, armoireRef, circuitRef)
		populateDriverFields(&item, drvBrand, drvModel, drvProtocol, dimmingProto, faultStatus, nomPowerW, outCurrentMA, outVoltageV, powerFactor, drvTemp, ledTemp, energyKWh, opHours, surgeProt, d4iCompat)
		result = append(result, item)
	}
	return result, nil
}

// ListLampadairesByLCU returns all lampadaires assigned to a given LCU.
func ListLampadairesByLCU(ctx context.Context, db *sql.DB, lcuID int) ([]models.Lampadaire, error) {
	query := `
		SELECT l.id, l.reference, l.latitude, l.longitude, l.zone, l.type_driver, l.protocole,
			l.puissance, l.etat, l.intensite, l.date_installation,
			l.last_seen_at, l.last_command_at, l.address, l.quartier, l.lcu_reference, l.driver_reference, l.notes,
			l.lcu_id, l.device_uid, l.node_address, l.discovered_by_lcu, l.location_status, l.commissioning_status,
			l.controller_uid, l.controller_type, l.controller_status, l.controller_signal_quality,
			l.controller_firmware, l.controller_last_seen_at, l.controller_embedded,
			l.dimming_enabled, l.metering_enabled, l.armoire_reference, l.circuit_reference,
			l.cabinet_id, l.circuit_id,
			l.driver_brand, l.driver_model, l.driver_protocol, l.nominal_power_w,
			l.output_current_ma, l.output_voltage_v, l.power_factor, l.surge_protection,
			l.dimming_protocol, l.d4i_compatible, l.driver_temperature, l.led_module_temperature,
			l.energy_kwh, l.operating_hours, l.fault_status,
			EXISTS(SELECT 1 FROM alerts a WHERE a.lampadaire_id = l.id AND a.status = 'open' AND a.severity = 'critical') as has_alert
		FROM lampadaires l
		WHERE l.lcu_id = $1 AND l.archived_at IS NULL
		ORDER BY l.id
	`
	rows, err := db.QueryContext(ctx, query, lcuID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.Lampadaire
	for rows.Next() {
		var item models.Lampadaire
		var zone, typeDriver, protocole sql.NullString
		var puissance, lid sql.NullInt64
		var lat, lng sql.NullFloat64
		var dateInstallation, lastSeenAt, lastCommandAt sql.NullTime
		var address, quartier, lcuReference, driverReference, notes, deviceUID, nodeAddress sql.NullString

		var ctrlUID, ctrlType, ctrlStatus, ctrlFirmware sql.NullString
		var ctrlSignal, cabID, circID sql.NullInt64
		var ctrlLastSeen sql.NullTime
		var ctrlEmbedded, dimmingEnabled, meteringEnabled sql.NullBool
		var armoireRef, circuitRef sql.NullString

		var drvBrand, drvModel, drvProtocol, dimmingProto, faultStatus sql.NullString
		var nomPowerW sql.NullInt64
		var outCurrentMA, outVoltageV, powerFactor, drvTemp, ledTemp, energyKWh, opHours sql.NullFloat64
		var surgeProt, d4iCompat sql.NullBool

		if err := rows.Scan(
			&item.ID, &item.Reference, &lat, &lng,
			&zone, &typeDriver, &protocole, &puissance,
			&item.Etat, &item.Intensite, &dateInstallation,
			&lastSeenAt, &lastCommandAt, &address, &quartier,
			&lcuReference, &driverReference, &notes,
			&lid, &deviceUID, &nodeAddress, &item.DiscoveredByLCU, &item.LocationStatus, &item.CommissioningStatus,
			&ctrlUID, &ctrlType, &ctrlStatus, &ctrlSignal,
			&ctrlFirmware, &ctrlLastSeen, &ctrlEmbedded,
			&dimmingEnabled, &meteringEnabled, &armoireRef, &circuitRef,
			&cabID, &circID,
			&drvBrand, &drvModel, &drvProtocol, &nomPowerW,
			&outCurrentMA, &outVoltageV, &powerFactor, &surgeProt,
			&dimmingProto, &d4iCompat, &drvTemp, &ledTemp,
			&energyKWh, &opHours, &faultStatus,
			&item.HasCriticalAlert,
		); err != nil {
			return nil, err
		}

		populateLampadaireFields(&item, lat, lng, zone, typeDriver, protocole, puissance, lid, dateInstallation, lastSeenAt, lastCommandAt, address, quartier, lcuReference, driverReference, notes, deviceUID, nodeAddress)
		populateControllerFields(&item, ctrlUID, ctrlType, ctrlStatus, ctrlFirmware, ctrlSignal, cabID, circID, ctrlLastSeen, ctrlEmbedded, dimmingEnabled, meteringEnabled, armoireRef, circuitRef)
		populateDriverFields(&item, drvBrand, drvModel, drvProtocol, dimmingProto, faultStatus, nomPowerW, outCurrentMA, outVoltageV, powerFactor, drvTemp, ledTemp, energyKWh, opHours, surgeProt, d4iCompat)
		result = append(result, item)
	}
	return result, nil
}

// MarkInactiveLampadairesOffline marks lampadaires that haven't sent data in >15min as offline.
func MarkInactiveLampadairesOffline(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		UPDATE lampadaires
		SET etat = 'offline', updated_at = NOW()
		WHERE archived_at IS NULL
		AND last_seen_at IS NOT NULL
		AND last_seen_at < NOW() - INTERVAL '15 minutes'
		AND etat != 'offline'
	`)
	if err != nil {
		return err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT id, reference FROM lampadaires
		WHERE archived_at IS NULL AND etat = 'offline'
		AND last_seen_at < NOW() - INTERVAL '15 minutes'
		AND NOT EXISTS (SELECT 1 FROM alerts WHERE lampadaire_id = lampadaires.id AND type = 'lampadaire_offline' AND status = 'open')
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int
			var ref string
			if err := rows.Scan(&id, &ref); err == nil {
				CreateAlertIfNotExists(ctx, db, id, "lampadaire_offline", "warning", "Aucune donnée reçue depuis plus de 15 minutes.")
			}
		}
	}

	return nil
}
