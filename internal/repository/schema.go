package repository

import (
	"context"
	"database/sql"
)

// DBExecutor defines common database operations for both *sql.DB and *sql.Tx.
type DBExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// EnsureSchema runs all DDL migrations.
func EnsureSchema(db *sql.DB) error {
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
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS commissioning_status TEXT NOT NULL DEFAULT 'discovered';

		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_commissioning_status') THEN
				ALTER TABLE lampadaires ADD CONSTRAINT chk_commissioning_status CHECK (commissioning_status IN ('discovered', 'located', 'configured', 'tested', 'commissioned'));
			END IF;
		END $$;

		CREATE TABLE IF NOT EXISTS lighting_groups (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			zone TEXT,
			description TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS lighting_profiles (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			target_type TEXT NOT NULL DEFAULT 'zone',
			target_value TEXT NOT NULL,
			enabled BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			CONSTRAINT chk_target_type CHECK (target_type IN ('zone', 'group', 'lcu'))
		);

		CREATE TABLE IF NOT EXISTS lighting_profile_schedules (
			id SERIAL PRIMARY KEY,
			profile_id INTEGER NOT NULL REFERENCES lighting_profiles(id) ON DELETE CASCADE,
			start_time TEXT NOT NULL,
			end_time TEXT NOT NULL,
			intensity INTEGER NOT NULL,
			days_of_week TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			CONSTRAINT chk_intensity CHECK (intensity BETWEEN 0 AND 100)
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
			closed_at TIMESTAMP NULL,
			CONSTRAINT chk_priority CHECK (priority IN ('low', 'medium', 'high', 'critical')),
			CONSTRAINT chk_status CHECK (status IN ('open', 'in_progress', 'resolved', 'closed'))
		);

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

		-- calculator_decisions columns added after initial schema
		ALTER TABLE calculator_decisions ADD COLUMN IF NOT EXISTS rule_name TEXT;
		ALTER TABLE calculator_decisions ADD COLUMN IF NOT EXISTS status TEXT DEFAULT 'pending';
		ALTER TABLE calculator_decisions ADD COLUMN IF NOT EXISTS validated_by INTEGER REFERENCES users(id);
		ALTER TABLE calculator_decisions ADD COLUMN IF NOT EXISTS validated_at TIMESTAMP NULL;
		ALTER TABLE calculator_decisions ADD COLUMN IF NOT EXISTS rejected_reason TEXT NULL;

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
			-- Drop and recreate check_alert_status to include 'in_progress'
			ALTER TABLE alerts DROP CONSTRAINT IF EXISTS check_alert_status;
			ALTER TABLE alerts ADD CONSTRAINT check_alert_status CHECK (status IN ('open', 'resolved', 'in_progress'));

			-- Dimming commands constraints
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'check_dimming_status') THEN
				ALTER TABLE dimming_commands ADD CONSTRAINT check_dimming_status CHECK (status IN ('pending', 'applied', 'failed'));
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'check_dimming_source') THEN
				ALTER TABLE dimming_commands ADD CONSTRAINT check_dimming_source CHECK (source IN ('admin', 'calculateur_intelligent', 'profile_eclairage', 'simulation'));
			END IF;
		END $$;
	`)
	if err != nil {
		return err
	}
	return ensureSchemaV2(db)
}

func ensureSchemaV2(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS basestations (
			id SERIAL PRIMARY KEY,
			reference VARCHAR(100) UNIQUE NOT NULL,
			name VARCHAR(200),
			latitude DOUBLE PRECISION,
			longitude DOUBLE PRECISION,
			zone VARCHAR(100),
			address TEXT,
			status VARCHAR(50) NOT NULL DEFAULT 'unknown',
			network_type VARCHAR(50) NOT NULL DEFAULT 'Simulated',
			primary_backhaul VARCHAR(50) NOT NULL DEFAULT 'simulated',
			active_backhaul VARCHAR(50),
			connected_nodes_count INT NOT NULL DEFAULT 0,
			disconnected_nodes_count INT NOT NULL DEFAULT 0,
			signal_quality_avg DOUBLE PRECISION NOT NULL DEFAULT 0,
			battery_status VARCHAR(50) NOT NULL DEFAULT 'unknown',
			last_seen_at TIMESTAMP WITH TIME ZONE,
			commissioned_at TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS cabinets (
			id SERIAL PRIMARY KEY,
			reference VARCHAR(100) UNIQUE NOT NULL,
			name VARCHAR(200),
			latitude DOUBLE PRECISION,
			longitude DOUBLE PRECISION,
			zone VARCHAR(100),
			address TEXT,
			status VARCHAR(50) NOT NULL DEFAULT 'unknown',
			door_status VARCHAR(50) NOT NULL DEFAULT 'unknown',
			power_status VARCHAR(50) NOT NULL DEFAULT 'normal',
			voltage_l1 DOUBLE PRECISION,
			voltage_l2 DOUBLE PRECISION,
			voltage_l3 DOUBLE PRECISION,
			current_l1 DOUBLE PRECISION,
			current_l2 DOUBLE PRECISION,
			current_l3 DOUBLE PRECISION,
			leakage_current DOUBLE PRECISION,
			energy_kwh DOUBLE PRECISION NOT NULL DEFAULT 0,
			last_seen_at TIMESTAMP WITH TIME ZONE,
			notes TEXT,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS cabinet_circuits (
			id SERIAL PRIMARY KEY,
			cabinet_id INT NOT NULL REFERENCES cabinets(id) ON DELETE CASCADE,
			name VARCHAR(100) NOT NULL,
			phase VARCHAR(10) NOT NULL DEFAULT 'L1',
			circuit_number INT NOT NULL DEFAULT 1,
			status VARCHAR(50) NOT NULL DEFAULT 'active',
			contactor_status VARCHAR(50) NOT NULL DEFAULT 'unknown',
			breaker_status VARCHAR(50) NOT NULL DEFAULT 'ok',
			measured_current DOUBLE PRECISION,
			measured_voltage DOUBLE PRECISION,
			measured_power DOUBLE PRECISION,
			lamp_count INT NOT NULL DEFAULT 0,
			profile_id INT REFERENCES lighting_profiles(id) ON DELETE SET NULL,
			last_fault_at TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS controllers (
			id SERIAL PRIMARY KEY,
			controller_uid VARCHAR(200) UNIQUE NOT NULL,
			serial_number VARCHAR(100),
			type VARCHAR(50) NOT NULL DEFAULT 'Simulated',
			lampadaire_id INT REFERENCES lampadaires(id) ON DELETE SET NULL,
			basestation_id INT REFERENCES basestations(id) ON DELETE SET NULL,
			cabinet_id INT REFERENCES cabinets(id) ON DELETE SET NULL,
			firmware_version VARCHAR(50),
			communication_status VARCHAR(50) NOT NULL DEFAULT 'ok',
			signal_quality INT NOT NULL DEFAULT 0,
			last_seen_at TIMESTAMP WITH TIME ZONE,
			metering_enabled BOOLEAN NOT NULL DEFAULT true,
			dimming_enabled BOOLEAN NOT NULL DEFAULT true,
			installation_status VARCHAR(50) NOT NULL DEFAULT 'discovered',
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS work_orders (
			id SERIAL PRIMARY KEY,
			title VARCHAR(300) NOT NULL,
			description TEXT,
			priority VARCHAR(50) NOT NULL DEFAULT 'medium',
			status VARCHAR(50) NOT NULL DEFAULT 'created',
			lampadaire_id INT REFERENCES lampadaires(id) ON DELETE SET NULL,
			cabinet_id INT REFERENCES cabinets(id) ON DELETE SET NULL,
			basestation_id INT REFERENCES basestations(id) ON DELETE SET NULL,
			circuit_id INT REFERENCES cabinet_circuits(id) ON DELETE SET NULL,
			assigned_to INT REFERENCES users(id) ON DELETE SET NULL,
			crew_type VARCHAR(50) NOT NULL DEFAULT 'lighting',
			due_date TIMESTAMP WITH TIME ZONE,
			probable_cause TEXT,
			resolution_note TEXT,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			closed_at TIMESTAMP WITH TIME ZONE
		);

		CREATE TABLE IF NOT EXISTS work_order_alerts (
			work_order_id INT NOT NULL REFERENCES work_orders(id) ON DELETE CASCADE,
			alert_id INT NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
			PRIMARY KEY (work_order_id, alert_id)
		);

		-- Alerts extended columns
		ALTER TABLE alerts ADD COLUMN IF NOT EXISTS source_type VARCHAR(50) NOT NULL DEFAULT 'lampadaire';
		ALTER TABLE alerts ADD COLUMN IF NOT EXISTS cabinet_id INT REFERENCES cabinets(id) ON DELETE SET NULL;
		ALTER TABLE alerts ADD COLUMN IF NOT EXISTS basestation_id INT REFERENCES basestations(id) ON DELETE SET NULL;
		ALTER TABLE alerts ADD COLUMN IF NOT EXISTS circuit_id INT REFERENCES cabinet_circuits(id) ON DELETE SET NULL;
		ALTER TABLE alerts ADD COLUMN IF NOT EXISTS acknowledged_at TIMESTAMP WITH TIME ZONE;
		ALTER TABLE alerts ADD COLUMN IF NOT EXISTS closed_at TIMESTAMP WITH TIME ZONE;
		ALTER TABLE alerts ADD COLUMN IF NOT EXISTS probable_cause TEXT;
		ALTER TABLE alerts ADD COLUMN IF NOT EXISTS recommended_action TEXT;

		-- Alerts: extend severity and status constraints to include new values
		ALTER TABLE alerts DROP CONSTRAINT IF EXISTS check_alert_severity;
		ALTER TABLE alerts ADD CONSTRAINT check_alert_severity CHECK (severity IN ('info', 'warning', 'major', 'critical'));
		ALTER TABLE alerts DROP CONSTRAINT IF EXISTS check_alert_status;
		ALTER TABLE alerts ADD CONSTRAINT check_alert_status CHECK (status IN ('open', 'acknowledged', 'in_progress', 'resolved', 'closed'));

		-- Commissioning workflow columns on lampadaires
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS commissioning_step INT NOT NULL DEFAULT 0;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS commissioned_at TIMESTAMP WITH TIME ZONE;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS commissioned_by INT REFERENCES users(id) ON DELETE SET NULL;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS test_comm_status VARCHAR(50) NOT NULL DEFAULT 'pending';
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS test_dimming_status VARCHAR(50) NOT NULL DEFAULT 'pending';
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS test_metering_status VARCHAR(50) NOT NULL DEFAULT 'pending';
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS commissioning_notes TEXT;

		-- Extend commissioning_status to include 'failed'
		ALTER TABLE lampadaires DROP CONSTRAINT IF EXISTS chk_commissioning_status;
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_commissioning_status_v2') THEN
				ALTER TABLE lampadaires ADD CONSTRAINT chk_commissioning_status_v2
					CHECK (commissioning_status IN ('discovered', 'located', 'configured', 'tested', 'commissioned', 'failed'));
			END IF;
		END $$;

		-- Indexes
		CREATE INDEX IF NOT EXISTS idx_basestations_status ON basestations(status);
		CREATE INDEX IF NOT EXISTS idx_basestations_zone ON basestations(zone);
		CREATE INDEX IF NOT EXISTS idx_cabinets_status ON cabinets(status);
		CREATE INDEX IF NOT EXISTS idx_cabinets_zone ON cabinets(zone);
		CREATE INDEX IF NOT EXISTS idx_controllers_lampadaire ON controllers(lampadaire_id);
		CREATE INDEX IF NOT EXISTS idx_controllers_basestation ON controllers(basestation_id);
		CREATE INDEX IF NOT EXISTS idx_work_orders_status ON work_orders(status);
		CREATE INDEX IF NOT EXISTS idx_work_orders_priority ON work_orders(priority);
		CREATE INDEX IF NOT EXISTS idx_alerts_source_type ON alerts(source_type);
		CREATE INDEX IF NOT EXISTS idx_alerts_severity ON alerts(severity);
		CREATE INDEX IF NOT EXISTS idx_alerts_status ON alerts(status);
	`)
	return err
}
