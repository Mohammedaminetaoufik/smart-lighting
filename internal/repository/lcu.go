package repository

import (
	"context"
	"database/sql"

	"map-interactif/internal/models"
	"map-interactif/internal/utils"
)

// InsertLCU inserts a new LCU.
func InsertLCU(ctx context.Context, db *sql.DB, lcu models.LCU) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO lcus (
			reference, name, ip_address, port, protocol, auth_token, zone, address, latitude, longitude, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`,
		lcu.Reference, utils.NullString(lcu.Name), lcu.IPAddress, lcu.Port, lcu.Protocol,
		utils.NullString(lcu.AuthToken), utils.NullString(lcu.Zone), utils.NullString(lcu.Address),
		lcu.Latitude, lcu.Longitude, lcu.Status,
	)
	return err
}

// UpsertLCUByReference inserts or updates an LCU by its reference.
func UpsertLCUByReference(ctx context.Context, db *sql.DB, lcu models.LCU) (models.LCU, error) {
	var result models.LCU
	var name, authToken, zone, address sql.NullString
	var lat, lng sql.NullFloat64
	var lastSeen, lastSync sql.NullTime
	err := db.QueryRowContext(ctx, `
		INSERT INTO lcus (reference, name, ip_address, port, protocol, auth_token, zone, address, latitude, longitude, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (reference) DO UPDATE SET
			name = EXCLUDED.name,
			ip_address = EXCLUDED.ip_address,
			port = EXCLUDED.port,
			protocol = EXCLUDED.protocol,
			zone = EXCLUDED.zone,
			address = EXCLUDED.address,
			updated_at = NOW()
		RETURNING id, reference, name, ip_address, port, protocol, auth_token, zone, address, latitude, longitude, status, last_seen_at, last_sync_at, created_at, updated_at
	`,
		lcu.Reference, utils.NullString(lcu.Name), lcu.IPAddress, lcu.Port, lcu.Protocol,
		utils.NullString(lcu.AuthToken), utils.NullString(lcu.Zone), utils.NullString(lcu.Address),
		lcu.Latitude, lcu.Longitude, lcu.Status,
	).Scan(
		&result.ID, &result.Reference, &name, &result.IPAddress, &result.Port, &result.Protocol,
		&authToken, &zone, &address, &lat, &lng, &result.Status, &lastSeen, &lastSync,
		&result.CreatedAt, &result.UpdatedAt,
	)
	if err != nil {
		return result, err
	}
	populateLCUNullFields(&result, name, authToken, zone, address, lat, lng, lastSeen, lastSync)
	return result, nil
}

// UpdateLCU updates an existing LCU.
func UpdateLCU(ctx context.Context, db *sql.DB, lcu models.LCU) error {
	_, err := db.ExecContext(ctx, `
		UPDATE lcus
		SET reference = $1,
			name = $2,
			ip_address = $3,
			port = $4,
			protocol = $5,
			auth_token = $6,
			zone = $7,
			address = $8,
			latitude = $9,
			longitude = $10,
			status = $11,
			updated_at = NOW()
		WHERE id = $12
	`,
		lcu.Reference, utils.NullString(lcu.Name), lcu.IPAddress, lcu.Port, lcu.Protocol,
		utils.NullString(lcu.AuthToken), utils.NullString(lcu.Zone), utils.NullString(lcu.Address),
		lcu.Latitude, lcu.Longitude, lcu.Status,
		lcu.ID,
	)
	return err
}

// GetLCUByID fetches a single LCU by its ID.
func GetLCUByID(ctx context.Context, db *sql.DB, id int) (*models.LCU, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, reference, name, ip_address, port, protocol, auth_token, zone, address, latitude, longitude, status, last_seen_at, last_sync_at, created_at, updated_at
		FROM lcus WHERE id = $1
	`, id)

	var l models.LCU
	var name, authToken, zone, address sql.NullString
	var lat, lng sql.NullFloat64
	var lastSeen, lastSync sql.NullTime

	err := row.Scan(
		&l.ID, &l.Reference, &name, &l.IPAddress, &l.Port, &l.Protocol, &authToken, &zone, &address, &lat, &lng, &l.Status, &lastSeen, &lastSync, &l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	populateLCUNullFields(&l, name, authToken, zone, address, lat, lng, lastSeen, lastSync)
	return &l, nil
}

// ListLCUs returns all LCUs ordered by reference.
func ListLCUs(ctx context.Context, db *sql.DB) ([]models.LCU, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, reference, name, ip_address, port, protocol, auth_token, zone, address, latitude, longitude, status, last_seen_at, last_sync_at, created_at, updated_at
		FROM lcus ORDER BY reference
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.LCU
	for rows.Next() {
		var l models.LCU
		var name, authToken, zone, address sql.NullString
		var lat, lng sql.NullFloat64
		var lastSeen, lastSync sql.NullTime

		err := rows.Scan(
			&l.ID, &l.Reference, &name, &l.IPAddress, &l.Port, &l.Protocol, &authToken, &zone, &address, &lat, &lng, &l.Status, &lastSeen, &lastSync, &l.CreatedAt, &l.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		populateLCUNullFields(&l, name, authToken, zone, address, lat, lng, lastSeen, lastSync)
		result = append(result, l)
	}
	return result, nil
}

// UpdateLCUStatus updates the status and last_seen_at of an LCU.
func UpdateLCUStatus(ctx context.Context, db *sql.DB, id int, status string) error {
	_, err := db.ExecContext(ctx, `UPDATE lcus SET status = $1, last_seen_at = NOW(), updated_at = NOW() WHERE id = $2`, status, id)
	return err
}

// InsertLCUSyncLog inserts an LCU sync log entry.
func InsertLCUSyncLog(ctx context.Context, db *sql.DB, log models.LCUSyncLog) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO lcu_sync_logs (lcu_id, status, message, discovered_count, created_count, updated_count, failed_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, log.LCUID, log.Status, log.Message, log.DiscoveredCount, log.CreatedCount, log.UpdatedCount, log.FailedCount)
	return err
}

// InsertLCUSyncLogTx inserts an LCU sync log entry within a transaction.
func InsertLCUSyncLogTx(ctx context.Context, tx *sql.Tx, log models.LCUSyncLog) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO lcu_sync_logs (lcu_id, status, message, discovered_count, created_count, updated_count, failed_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, log.LCUID, log.Status, log.Message, log.DiscoveredCount, log.CreatedCount, log.UpdatedCount, log.FailedCount)
	return err
}

func populateLCUNullFields(l *models.LCU, name, authToken, zone, address sql.NullString, lat, lng sql.NullFloat64, lastSeen, lastSync sql.NullTime) {
	if name.Valid {
		l.Name = name.String
	}
	if authToken.Valid {
		l.AuthToken = authToken.String
	}
	if zone.Valid {
		l.Zone = zone.String
	}
	if address.Valid {
		l.Address = address.String
	}
	if lat.Valid {
		l.Latitude = &lat.Float64
	}
	if lng.Valid {
		l.Longitude = &lng.Float64
	}
	if lastSeen.Valid {
		l.LastSeenAt = &lastSeen.Time
	}
	if lastSync.Valid {
		l.LastSyncAt = &lastSync.Time
	}
}
