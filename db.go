package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
)

func openDB() (*sql.DB, error) {
	dsn, err := buildDSN()
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		if !isMissingDatabaseError(err) {
			return nil, err
		}

		if err := createDatabase(); err != nil {
			return nil, err
		}

		if err := db.Ping(); err != nil {
			return nil, err
		}
	}

	return db, nil
}

func isMissingDatabaseError(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()
	if strings.Contains(message, "SQLSTATE 3D000") {
		return true
	}

	return strings.Contains(message, "database") && strings.Contains(message, "does not exist")
}

func createDatabase() error {
	host := strings.TrimSpace(os.Getenv("DB_HOST"))
	port := strings.TrimSpace(os.Getenv("DB_PORT"))
	user := strings.TrimSpace(os.Getenv("DB_USER"))
	password := os.Getenv("DB_PASSWORD")
	name := strings.TrimSpace(os.Getenv("DB_NAME"))
	sslmode := strings.TrimSpace(os.Getenv("DB_SSLMODE"))

	if host == "" || user == "" || name == "" {
		return fmt.Errorf("DB_HOST, DB_USER, and DB_NAME must be set to create the database")
	}

	if port == "" {
		port = "5432"
	}
	if sslmode == "" {
		sslmode = "disable"
	}

	adminURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   fmt.Sprintf("%s:%s", host, port),
		Path:   "postgres",
	}

	query := adminURL.Query()
	query.Set("sslmode", sslmode)
	adminURL.RawQuery = query.Encode()

	adminDB, err := sql.Open("pgx", adminURL.String())
	if err != nil {
		return err
	}
	defer adminDB.Close()

	if err := adminDB.Ping(); err != nil {
		return err
	}

	_, err = adminDB.Exec("CREATE DATABASE " + quoteIdentifier(name))
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return nil
		}
		return err
	}

	return nil
}

func quoteIdentifier(value string) string {
	return "\"" + strings.ReplaceAll(value, "\"", "\"\"") + "\""
}

func buildDSN() (string, error) {
	if value := strings.TrimSpace(os.Getenv("DATABASE_URL")); value != "" {
		return value, nil
	}

	host := strings.TrimSpace(os.Getenv("DB_HOST"))
	port := strings.TrimSpace(os.Getenv("DB_PORT"))
	user := strings.TrimSpace(os.Getenv("DB_USER"))
	password := os.Getenv("DB_PASSWORD")
	name := strings.TrimSpace(os.Getenv("DB_NAME"))
	sslmode := strings.TrimSpace(os.Getenv("DB_SSLMODE"))

	if host == "" || user == "" || name == "" {
		return "", fmt.Errorf("DATABASE_URL or DB_HOST/DB_USER/DB_NAME must be set")
	}

	if port == "" {
		port = "5432"
	}
	if sslmode == "" {
		sslmode = "disable"
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   fmt.Sprintf("%s:%s", host, port),
		Path:   name,
	}

	query := u.Query()
	query.Set("sslmode", sslmode)
	u.RawQuery = query.Encode()

	return u.String(), nil
}

func ensureSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS lcus (
			id SERIAL PRIMARY KEY,
			reference TEXT NOT NULL UNIQUE,
			name TEXT,
			ip_address TEXT NOT NULL,
			port INTEGER NOT NULL DEFAULT 8080,
			protocol TEXT NOT NULL DEFAULT 'HTTP',
			auth_token TEXT,
			zone TEXT,
			address TEXT,
			latitude DOUBLE PRECISION,
			longitude DOUBLE PRECISION,
			status TEXT NOT NULL DEFAULT 'offline',
			last_seen_at TIMESTAMP NULL,
			last_sync_at TIMESTAMP NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS lcu_sync_logs (
			id SERIAL PRIMARY KEY,
			lcu_id INTEGER NOT NULL REFERENCES lcus(id) ON DELETE CASCADE,
			status TEXT NOT NULL,
			message TEXT,
			discovered_count INTEGER NOT NULL DEFAULT 0,
			created_count INTEGER NOT NULL DEFAULT 0,
			updated_count INTEGER NOT NULL DEFAULT 0,
			failed_count INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS lampadaires (
			id SERIAL PRIMARY KEY,
			reference TEXT NOT NULL,
			latitude DOUBLE PRECISION NOT NULL,
			longitude DOUBLE PRECISION NOT NULL,
			zone TEXT,
			type_driver TEXT,
			protocole TEXT,
			puissance INTEGER,
			etat TEXT NOT NULL DEFAULT 'offline',
			intensite INTEGER NOT NULL DEFAULT 0,
			date_installation DATE,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS archived_at TIMESTAMP NULL;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMP NULL;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS last_command_at TIMESTAMP NULL;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS address TEXT NULL;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS quartier TEXT NULL;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS lcu_reference TEXT NULL;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS driver_reference TEXT NULL;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS notes TEXT NULL;

		-- Professional IoT columns
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS lcu_id INTEGER REFERENCES lcus(id);
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS device_uid TEXT;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS node_address TEXT;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS discovered_by_lcu BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS location_status TEXT NOT NULL DEFAULT 'manual';

		CREATE UNIQUE INDEX IF NOT EXISTS idx_lampadaire_lcu_device
		ON lampadaires(lcu_id, device_uid)
		WHERE device_uid IS NOT NULL;

		ALTER TABLE lampadaires ALTER COLUMN latitude DROP NOT NULL;
		ALTER TABLE lampadaires ALTER COLUMN longitude DROP NOT NULL;

		CREATE TABLE IF NOT EXISTS sensor_measurements (
			id SERIAL PRIMARY KEY,
			lampadaire_id INTEGER NOT NULL REFERENCES lampadaires(id) ON DELETE CASCADE,
			luminosite DOUBLE PRECISION,
			presence BOOLEAN,
			temperature DOUBLE PRECISION,
			humidite DOUBLE PRECISION,
			tension DOUBLE PRECISION,
			courant DOUBLE PRECISION,
			puissance DOUBLE PRECISION,
			energie DOUBLE PRECISION,
			source TEXT NOT NULL DEFAULT 'simulation',
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS dimming_commands (
			id SERIAL PRIMARY KEY,
			lampadaire_id INTEGER NOT NULL REFERENCES lampadaires(id) ON DELETE CASCADE,
			source TEXT NOT NULL,
			old_intensity INTEGER,
			new_intensity INTEGER NOT NULL,
			reason TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			applied_at TIMESTAMP NULL
		);

		CREATE TABLE IF NOT EXISTS alerts (
			id SERIAL PRIMARY KEY,
			lampadaire_id INTEGER REFERENCES lampadaires(id) ON DELETE CASCADE,
			type TEXT NOT NULL,
			severity TEXT NOT NULL,
			message TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'open',
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			resolved_at TIMESTAMP NULL
		);

		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			full_name TEXT NOT NULL,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT,
			role TEXT NOT NULL DEFAULT 'admin',
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS calculator_decisions (
			id SERIAL PRIMARY KEY,
			lampadaire_id INTEGER NOT NULL REFERENCES lampadaires(id) ON DELETE CASCADE,
			recommended_intensity INTEGER NOT NULL,
			decision_reason TEXT NOT NULL,
			confidence DOUBLE PRECISION NOT NULL DEFAULT 1.0,
			applied BOOLEAN NOT NULL DEFAULT false,
			rule_name TEXT,
			status TEXT DEFAULT 'pending',
			validated_by INTEGER REFERENCES users(id),
			validated_at TIMESTAMP NULL,
			rejected_reason TEXT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS access_logs (
			id SERIAL PRIMARY KEY,
			user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
			action TEXT NOT NULL,
			ip_address TEXT,
			user_agent TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS system_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS interventions (
			id SERIAL PRIMARY KEY,
			alert_id INTEGER REFERENCES alerts(id) ON DELETE SET NULL,
			lampadaire_id INTEGER REFERENCES lampadaires(id) ON DELETE CASCADE,
			assigned_to INTEGER REFERENCES users(id) ON DELETE SET NULL,
			title TEXT NOT NULL,
			description TEXT,
			priority TEXT NOT NULL DEFAULT 'medium',
			status TEXT NOT NULL DEFAULT 'open',
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			closed_at TIMESTAMP NULL
		);

		-- Professionnal Constraints
		DO $$ 
		BEGIN 
			-- Lampadaires constraints
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'check_lampadaire_etat') THEN
				ALTER TABLE lampadaires ADD CONSTRAINT check_lampadaire_etat CHECK (etat IN ('online', 'offline', 'maintenance'));
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'check_lampadaire_intensite') THEN
				ALTER TABLE lampadaires ADD CONSTRAINT check_lampadaire_intensite CHECK (intensite >= 0 AND intensite <= 100);
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'check_lampadaire_location_status') THEN
				ALTER TABLE lampadaires ADD CONSTRAINT check_lampadaire_location_status CHECK (location_status IN ('confirmed', 'missing', 'estimated', 'manual'));
			END IF;

			-- LCU constraints
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'check_lcu_status') THEN
				ALTER TABLE lcus ADD CONSTRAINT check_lcu_status CHECK (status IN ('online', 'offline', 'maintenance', 'unknown'));
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'check_lcu_protocol') THEN
				ALTER TABLE lcus ADD CONSTRAINT check_lcu_protocol CHECK (protocol IN ('HTTP', 'MQTT', 'ModbusTCP', 'LoRaWAN', 'ZigBee', 'PLC'));
			END IF;

			-- Alerts constraints
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'check_alert_severity') THEN
				ALTER TABLE alerts ADD CONSTRAINT check_alert_severity CHECK (severity IN ('info', 'warning', 'critical'));
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'check_alert_status') THEN
				ALTER TABLE alerts ADD CONSTRAINT check_alert_status CHECK (status IN ('open', 'resolved'));
			END IF;

			-- Dimming commands constraints
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'check_dimming_status') THEN
				ALTER TABLE dimming_commands ADD CONSTRAINT check_dimming_status CHECK (status IN ('pending', 'applied', 'failed'));
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'check_dimming_source') THEN
				ALTER TABLE dimming_commands ADD CONSTRAINT check_dimming_source CHECK (source IN ('admin', 'calculateur_intelligent', 'profile_eclairage', 'simulation'));
			END IF;
		END $$;
	`)
	return err
}

// getLampadaireByID fetches a single lampadaire by its ID.
func getLampadaireByID(ctx context.Context, db *sql.DB, id int) (*Lampadaire, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, reference, latitude, longitude, zone, type_driver, protocole,
			puissance, etat, intensite, date_installation,
			last_seen_at, last_command_at, address, quartier, lcu_reference, driver_reference, notes,
			lcu_id, device_uid, node_address, discovered_by_lcu, location_status
		FROM lampadaires
		WHERE id = $1 AND archived_at IS NULL
	`, id)

	var item Lampadaire
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
		&lcuID, &deviceUID, &nodeAddress, &item.DiscoveredByLCU, &item.LocationStatus,
	)
	if err != nil {
		return nil, err
	}

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

	return &item, nil
}

// listLampadaires returns all non-archived lampadaires matching optional filters.
func listLampadaires(ctx context.Context, db *sql.DB, search map[string]string) ([]Lampadaire, error) {
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

	query := `
		SELECT l.id, l.reference, l.latitude, l.longitude, l.zone, l.type_driver, l.protocole,
			l.puissance, l.etat, l.intensite, l.date_installation,
			l.last_seen_at, l.last_command_at, l.address, l.quartier, l.lcu_reference, l.driver_reference, l.notes,
			l.lcu_id, l.device_uid, l.node_address, l.discovered_by_lcu, l.location_status,
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

	var result []Lampadaire
	for rows.Next() {
		var item Lampadaire
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
			&lcuID, &deviceUID, &nodeAddress, &item.DiscoveredByLCU, &item.LocationStatus,
			&item.HasCriticalAlert,
		); err != nil {
			return nil, err
		}

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

		result = append(result, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func insertLampadaire(ctx context.Context, db *sql.DB, l Lampadaire) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO lampadaires (
			reference, latitude, longitude, zone, type_driver, protocole, puissance,
			etat, intensite, date_installation, address, quartier, lcu_reference,
			driver_reference, notes, lcu_id, device_uid, node_address, discovered_by_lcu, location_status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
	`,
		l.Reference, l.Latitude, l.Longitude,
		nullString(l.Zone), nullString(l.TypeDriver), nullString(l.Protocole),
		l.Puissance, l.Etat, l.Intensite, l.DateInstallation,
		nullString(l.Address), nullString(l.Quartier), nullString(l.LCUReference),
		nullString(l.DriverReference), nullString(l.Notes),
		l.LCUID, nullString(l.DeviceUID), nullString(l.NodeAddress), l.DiscoveredByLCU, l.LocationStatus,
	)
	return err
}

func insertLampadaireTx(ctx context.Context, tx *sql.Tx, l Lampadaire) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO lampadaires (
			reference, latitude, longitude, zone, type_driver, protocole, puissance,
			etat, intensite, date_installation, address, quartier, lcu_reference,
			driver_reference, notes, lcu_id, device_uid, node_address, discovered_by_lcu, location_status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
	`,
		l.Reference, l.Latitude, l.Longitude,
		nullString(l.Zone), nullString(l.TypeDriver), nullString(l.Protocole),
		l.Puissance, l.Etat, l.Intensite, l.DateInstallation,
		nullString(l.Address), nullString(l.Quartier), nullString(l.LCUReference),
		nullString(l.DriverReference), nullString(l.Notes),
		l.LCUID, nullString(l.DeviceUID), nullString(l.NodeAddress), l.DiscoveredByLCU, l.LocationStatus,
	)
	return err
}

func updateLampadaire(ctx context.Context, db *sql.DB, l Lampadaire) error {
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
			updated_at = NOW()
		WHERE id = $21
	`,
		l.Reference, l.Latitude, l.Longitude,
		nullString(l.Zone), nullString(l.TypeDriver), nullString(l.Protocole),
		l.Puissance, l.Etat, l.Intensite, l.DateInstallation,
		nullString(l.Address), nullString(l.Quartier), nullString(l.LCUReference),
		nullString(l.DriverReference), nullString(l.Notes),
		l.LCUID, nullString(l.DeviceUID), nullString(l.NodeAddress), l.DiscoveredByLCU, l.LocationStatus,
		l.ID,
	)
	return err
}

func updateLampadaireTx(ctx context.Context, tx *sql.Tx, l Lampadaire) error {
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
			updated_at = NOW()
		WHERE id = $21
	`,
		l.Reference, l.Latitude, l.Longitude,
		nullString(l.Zone), nullString(l.TypeDriver), nullString(l.Protocole),
		l.Puissance, l.Etat, l.Intensite, l.DateInstallation,
		nullString(l.Address), nullString(l.Quartier), nullString(l.LCUReference),
		nullString(l.DriverReference), nullString(l.Notes),
		l.LCUID, nullString(l.DeviceUID), nullString(l.NodeAddress), l.DiscoveredByLCU, l.LocationStatus,
		l.ID,
	)
	return err
}

// LCU Functions

func insertLCU(ctx context.Context, db *sql.DB, lcu LCU) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO lcus (
			reference, name, ip_address, port, protocol, auth_token, zone, address, latitude, longitude, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`,
		lcu.Reference, nullString(lcu.Name), lcu.IPAddress, lcu.Port, lcu.Protocol,
		nullString(lcu.AuthToken), nullString(lcu.Zone), nullString(lcu.Address),
		lcu.Latitude, lcu.Longitude, lcu.Status,
	)
	return err
}

func updateLCU(ctx context.Context, db *sql.DB, lcu LCU) error {
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
		lcu.Reference, nullString(lcu.Name), lcu.IPAddress, lcu.Port, lcu.Protocol,
		nullString(lcu.AuthToken), nullString(lcu.Zone), nullString(lcu.Address),
		lcu.Latitude, lcu.Longitude, lcu.Status,
		lcu.ID,
	)
	return err
}

func getLCUByID(ctx context.Context, db *sql.DB, id int) (*LCU, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, reference, name, ip_address, port, protocol, auth_token, zone, address, latitude, longitude, status, last_seen_at, last_sync_at, created_at, updated_at
		FROM lcus WHERE id = $1
	`, id)

	var l LCU
	var name, authToken, zone, address sql.NullString
	var lat, lng sql.NullFloat64
	var lastSeen, lastSync sql.NullTime

	err := row.Scan(
		&l.ID, &l.Reference, &name, &l.IPAddress, &l.Port, &l.Protocol, &authToken, &zone, &address, &lat, &lng, &l.Status, &lastSeen, &lastSync, &l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

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

	return &l, nil
}

func listLCUs(ctx context.Context, db *sql.DB) ([]LCU, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, reference, name, ip_address, port, protocol, auth_token, zone, address, latitude, longitude, status, last_seen_at, last_sync_at, created_at, updated_at
		FROM lcus ORDER BY reference
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []LCU
	for rows.Next() {
		var l LCU
		var name, authToken, zone, address sql.NullString
		var lat, lng sql.NullFloat64
		var lastSeen, lastSync sql.NullTime

		err := rows.Scan(
			&l.ID, &l.Reference, &name, &l.IPAddress, &l.Port, &l.Protocol, &authToken, &zone, &address, &lat, &lng, &l.Status, &lastSeen, &lastSync, &l.CreatedAt, &l.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

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

		result = append(result, l)
	}
	return result, nil
}

func updateLCUStatus(ctx context.Context, db *sql.DB, id int, status string) error {
	_, err := db.ExecContext(ctx, `UPDATE lcus SET status = $1, last_seen_at = NOW(), updated_at = NOW() WHERE id = $2`, status, id)
	return err
}

func insertLCUSyncLog(ctx context.Context, db *sql.DB, log LCUSyncLog) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO lcu_sync_logs (lcu_id, status, message, discovered_count, created_count, updated_count, failed_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, log.LCUID, log.Status, log.Message, log.DiscoveredCount, log.CreatedCount, log.UpdatedCount, log.FailedCount)
	return err
}

func insertLCUSyncLogTx(ctx context.Context, tx *sql.Tx, log LCUSyncLog) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO lcu_sync_logs (lcu_id, status, message, discovered_count, created_count, updated_count, failed_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, log.LCUID, log.Status, log.Message, log.DiscoveredCount, log.CreatedCount, log.UpdatedCount, log.FailedCount)
	return err
}

func listLampadairesByLCU(ctx context.Context, db *sql.DB, lcuID int) ([]Lampadaire, error) {

	// We'll modify listLampadaires to handle lcu_id filter or just write a specialized query
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

	var result []Lampadaire
	for rows.Next() {
		var item Lampadaire
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
		if lid.Valid {
			v := int(lid.Int64)
			item.LCUID = &v
		}
		if deviceUID.Valid {
			item.DeviceUID = deviceUID.String
		}
		if nodeAddress.Valid {
			item.NodeAddress = nodeAddress.String
		}

		result = append(result, item)
	}
	return result, nil
}

func listLampadairesMissingLocation(ctx context.Context, db *sql.DB) ([]Lampadaire, error) {
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

	var result []Lampadaire
	for rows.Next() {
		var item Lampadaire
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
		if lid.Valid {
			v := int(lid.Int64)
			item.LCUID = &v
		}
		if deviceUID.Valid {
			item.DeviceUID = deviceUID.String
		}
		if nodeAddress.Valid {
			item.NodeAddress = nodeAddress.String
		}

		result = append(result, item)
	}
	return result, nil
}

func updateLampadaireLocation(ctx context.Context, db *sql.DB, id int, lat, lng float64, status string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE lampadaires SET latitude = $1, longitude = $2, location_status = $3, updated_at = NOW() WHERE id = $4
	`, lat, lng, status, id)
	return err
}

func markInactiveLampadairesOffline(ctx context.Context, db *sql.DB) error {
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

	// Add alert for newly offline lampadaires
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
				createAlertIfNotExists(ctx, db, id, "lampadaire_offline", "warning", "Aucune donnée reçue depuis plus de 15 minutes.")
			}
		}
	}

	return nil
}


