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
			lcu_id, device_uid, node_address, discovered_by_lcu, location_status, commissioning_status
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

	err := row.Scan(
		&item.ID, &item.Reference, &lat, &lng,
		&zone, &typeDriver, &protocole, &puissance,
		&item.Etat, &item.Intensite, &dateInstallation,
		&lastSeenAt, &lastCommandAt, &address, &quartier,
		&lcuReference, &driverReference, &notes,
		&lcuID, &deviceUID, &nodeAddress, &item.DiscoveredByLCU, &item.LocationStatus, &item.CommissioningStatus,
	)
	if err != nil {
		return nil, err
	}

	populateLampadaireFields(&item, lat, lng, zone, typeDriver, protocole, puissance, lcuID, dateInstallation, lastSeenAt, lastCommandAt, address, quartier, lcuReference, driverReference, notes, deviceUID, nodeAddress)
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

// ListLampadaires returns all non-archived lampadaires matching optional filters.
func ListLampadaires(ctx context.Context, db *sql.DB, search map[string]string) ([]models.Lampadaire, error) {
	archivedOnly := search["archived"] == "1"
	where := []string{"archived_at IS NULL"}
	if archivedOnly {
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
	if q := search["q"]; q != "" {
		where = append(where, fmt.Sprintf("(reference ILIKE $%d OR address ILIKE $%d OR quartier ILIKE $%d)", argID, argID, argID))
		args = append(args, "%"+q+"%")
		argID++
	}
	_ = argID

	query := `
		SELECT l.id, l.reference, l.latitude, l.longitude, l.zone, l.type_driver, l.protocole,
			l.puissance, l.etat, l.intensite, l.date_installation,
			l.last_seen_at, l.last_command_at, l.address, l.quartier, l.lcu_reference, l.driver_reference, l.notes,
			l.lcu_id, l.device_uid, l.node_address, l.discovered_by_lcu, l.location_status, l.commissioning_status,
			EXISTS(SELECT 1 FROM alerts a WHERE a.lampadaire_id = l.id AND a.status = 'open' AND a.severity = 'critical') as has_alert
		FROM lampadaires l
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY l.id
	`

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

		if err := rows.Scan(
			&item.ID, &item.Reference, &lat, &lng,
			&zone, &typeDriver, &protocole, &puissance,
			&item.Etat, &item.Intensite, &dateInstallation,
			&lastSeenAt, &lastCommandAt, &address, &quartier,
			&lcuReference, &driverReference, &notes,
			&lcuID, &deviceUID, &nodeAddress, &item.DiscoveredByLCU, &item.LocationStatus, &item.CommissioningStatus,
			&item.HasCriticalAlert,
		); err != nil {
			return nil, err
		}

		populateLampadaireFields(&item, lat, lng, zone, typeDriver, protocole, puissance, lcuID, dateInstallation, lastSeenAt, lastCommandAt, address, quartier, lcuReference, driverReference, notes, deviceUID, nodeAddress)
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
			driver_reference, notes, lcu_id, device_uid, node_address, discovered_by_lcu, location_status, commissioning_status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
	`,
		l.Reference, l.Latitude, l.Longitude,
		utils.NullString(l.Zone), utils.NullString(l.TypeDriver), utils.NullString(l.Protocole),
		l.Puissance, l.Etat, l.Intensite, l.DateInstallation,
		utils.NullString(l.Address), utils.NullString(l.Quartier), utils.NullString(l.LCUReference),
		utils.NullString(l.DriverReference), utils.NullString(l.Notes),
		l.LCUID, utils.NullString(l.DeviceUID), utils.NullString(l.NodeAddress), l.DiscoveredByLCU, l.LocationStatus, l.CommissioningStatus,
	)
	return err
}

// InsertLampadaireTx inserts a new lampadaire within a transaction.
func InsertLampadaireTx(ctx context.Context, tx *sql.Tx, l models.Lampadaire) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO lampadaires (
			reference, latitude, longitude, zone, type_driver, protocole, puissance,
			etat, intensite, date_installation, address, quartier, lcu_reference,
			driver_reference, notes, lcu_id, device_uid, node_address, discovered_by_lcu, location_status, commissioning_status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
	`,
		l.Reference, l.Latitude, l.Longitude,
		utils.NullString(l.Zone), utils.NullString(l.TypeDriver), utils.NullString(l.Protocole),
		l.Puissance, l.Etat, l.Intensite, l.DateInstallation,
		utils.NullString(l.Address), utils.NullString(l.Quartier), utils.NullString(l.LCUReference),
		utils.NullString(l.DriverReference), utils.NullString(l.Notes),
		l.LCUID, utils.NullString(l.DeviceUID), utils.NullString(l.NodeAddress), l.DiscoveredByLCU, l.LocationStatus, l.CommissioningStatus,
	)
	return err
}

// UpdateLampadaire updates an existing lampadaire.
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

// UpdateLampadaireTx updates an existing lampadaire within a transaction.
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

// ListLampadairesMissingLocation returns lampadaires with missing or no coordinates.
func ListLampadairesMissingLocation(ctx context.Context, db *sql.DB) ([]models.Lampadaire, error) {
	query := `
		SELECT l.id, l.reference, l.latitude, l.longitude, l.zone, l.type_driver, l.protocole,
			l.puissance, l.etat, l.intensite, l.date_installation,
			l.last_seen_at, l.last_command_at, l.address, l.quartier, l.lcu_reference, l.driver_reference, l.notes,
			l.lcu_id, l.device_uid, l.node_address, l.discovered_by_lcu, l.location_status,
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

		if err := rows.Scan(
			&item.ID, &item.Reference, &lat, &lng,
			&zone, &typeDriver, &protocole, &puissance,
			&item.Etat, &item.Intensite, &dateInstallation,
			&lastSeenAt, &lastCommandAt, &address, &quartier,
			&lcuReference, &driverReference, &notes,
			&lid, &deviceUID, &nodeAddress, &item.DiscoveredByLCU, &item.LocationStatus,
			&item.HasCriticalAlert,
		); err != nil {
			return nil, err
		}

		populateLampadaireFields(&item, lat, lng, zone, typeDriver, protocole, puissance, lid, dateInstallation, lastSeenAt, lastCommandAt, address, quartier, lcuReference, driverReference, notes, deviceUID, nodeAddress)
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
			l.lcu_id, l.device_uid, l.node_address, l.discovered_by_lcu, l.location_status,
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

		if err := rows.Scan(
			&item.ID, &item.Reference, &lat, &lng,
			&zone, &typeDriver, &protocole, &puissance,
			&item.Etat, &item.Intensite, &dateInstallation,
			&lastSeenAt, &lastCommandAt, &address, &quartier,
			&lcuReference, &driverReference, &notes,
			&lid, &deviceUID, &nodeAddress, &item.DiscoveredByLCU, &item.LocationStatus,
			&item.HasCriticalAlert,
		); err != nil {
			return nil, err
		}

		populateLampadaireFields(&item, lat, lng, zone, typeDriver, protocole, puissance, lid, dateInstallation, lastSeenAt, lastCommandAt, address, quartier, lcuReference, driverReference, notes, deviceUID, nodeAddress)
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
