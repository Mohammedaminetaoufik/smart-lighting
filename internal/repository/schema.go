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

		-- Controller fields embedded in lampadaire (detected automatically during LCU sync)
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS controller_uid TEXT;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS controller_type TEXT;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS controller_status TEXT NOT NULL DEFAULT 'unknown';
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS controller_signal_quality INTEGER;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS controller_firmware TEXT;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS controller_last_seen_at TIMESTAMP NULL;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS controller_embedded BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS dimming_enabled BOOLEAN NOT NULL DEFAULT true;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS metering_enabled BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS armoire_reference TEXT;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS circuit_reference TEXT;

		CREATE INDEX IF NOT EXISTS idx_lampadaires_controller_uid ON lampadaires(controller_uid) WHERE controller_uid IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_lampadaires_controller_status ON lampadaires(controller_status);
		CREATE INDEX IF NOT EXISTS idx_lampadaires_armoire_ref ON lampadaires(armoire_reference) WHERE armoire_reference IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_lampadaires_circuit_ref ON lampadaires(circuit_reference) WHERE circuit_reference IS NOT NULL;

		-- chk_commissioning_status is fully managed in ensureSchemaV2 (includes 'failed')

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
			-- check_alert_status is fully redefined in ensureSchemaV2 with all valid values

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

		-- FK references to cabinets/circuits for lampadaires (after those tables are created)
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS cabinet_id INTEGER REFERENCES cabinets(id) ON DELETE SET NULL;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS circuit_id INTEGER REFERENCES cabinet_circuits(id) ON DELETE SET NULL;

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

		-- Job heartbeats (Tier 4 observability)
		CREATE TABLE IF NOT EXISTS job_heartbeats (
			name TEXT PRIMARY KEY,
			last_run_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			status TEXT NOT NULL DEFAULT 'ok',
			message TEXT,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		);

		-- Maintenance windows (Tier 4)
		CREATE TABLE IF NOT EXISTS maintenance_windows (
			id SERIAL PRIMARY KEY,
			zone TEXT,
			lampadaire_ids JSONB,
			start_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			end_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			reason TEXT,
			created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_maintenance_active ON maintenance_windows(start_at, end_at);

		-- Users auth extensions (Tier 1)
		ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMP WITH TIME ZONE;
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'check_user_role') THEN
				ALTER TABLE users ADD CONSTRAINT check_user_role CHECK (role IN ('admin', 'operator', 'viewer'));
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'check_user_status') THEN
				ALTER TABLE users ADD CONSTRAINT check_user_status CHECK (status IN ('active', 'disabled', 'deleted'));
			END IF;
		END $$;

		-- Audit log extensions (Tier 2)
		ALTER TABLE access_logs ADD COLUMN IF NOT EXISTS target_type TEXT;
		ALTER TABLE access_logs ADD COLUMN IF NOT EXISTS target_id INTEGER;
		ALTER TABLE access_logs ADD COLUMN IF NOT EXISTS metadata JSONB;

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
		CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
		CREATE INDEX IF NOT EXISTS idx_access_logs_user ON access_logs(user_id);
		CREATE INDEX IF NOT EXISTS idx_access_logs_created ON access_logs(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_access_logs_action ON access_logs(action);

		-- Tier 4 Schema Extensions
		CREATE TABLE IF NOT EXISTS job_heartbeats (
			name TEXT PRIMARY KEY,
			last_run_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			status TEXT NOT NULL
		);

		ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS updated_by INTEGER REFERENCES users(id);

		CREATE TABLE IF NOT EXISTS maintenance_windows (
			id SERIAL PRIMARY KEY,
			zone TEXT,
			lampadaire_ids JSONB,
			start_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			end_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			reason TEXT,
			created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		);

		-- Driver fields for Zhaga Book 18 and other embedded drivers
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS driver_brand TEXT;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS driver_model TEXT;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS driver_protocol TEXT;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS nominal_power_w INTEGER;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS output_current_ma DOUBLE PRECISION;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS output_voltage_v DOUBLE PRECISION;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS power_factor DOUBLE PRECISION;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS surge_protection BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS dimming_protocol TEXT;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS d4i_compatible BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS driver_temperature DOUBLE PRECISION;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS led_module_temperature DOUBLE PRECISION;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS energy_kwh DOUBLE PRECISION;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS operating_hours DOUBLE PRECISION;
		ALTER TABLE lampadaires ADD COLUMN IF NOT EXISTS fault_status TEXT NOT NULL DEFAULT 'none';

		CREATE TABLE IF NOT EXISTS alerts_archive (
			id INTEGER PRIMARY KEY,
			lampadaire_id INTEGER,
			type TEXT,
			severity TEXT,
			message TEXT,
			status TEXT,
			created_at TIMESTAMP,
			resolved_at TIMESTAMP,
			source_type TEXT,
			cabinet_id INTEGER,
			basestation_id INTEGER,
			circuit_id INTEGER,
			acknowledged_at TIMESTAMP,
			closed_at TIMESTAMP,
			probable_cause TEXT,
			recommended_action TEXT,
			archived_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		);

		-- Smart work order / maintenance workflow (v3)
		ALTER TABLE alerts ADD COLUMN IF NOT EXISTS work_order_id INT REFERENCES work_orders(id) ON DELETE SET NULL;

		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS source_type VARCHAR(50) NOT NULL DEFAULT 'manual';
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS source_alert_id INT REFERENCES alerts(id) ON DELETE SET NULL;
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS lcu_id INT REFERENCES lcus(id) ON DELETE SET NULL;
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS zone TEXT;
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS equipment_type VARCHAR(50) NOT NULL DEFAULT 'lampadaire';
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS equipment_reference TEXT;
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS recommended_action TEXT;
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS team_type VARCHAR(50) NOT NULL DEFAULT 'lighting';
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMP WITH TIME ZONE;
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS repeat_count INT NOT NULL DEFAULT 1;

		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_work_order_source') THEN
				ALTER TABLE work_orders ADD CONSTRAINT chk_work_order_source
					CHECK (source_type IN ('alert', 'manual', 'system'));
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_work_order_equipment') THEN
				ALTER TABLE work_orders ADD CONSTRAINT chk_work_order_equipment
					CHECK (equipment_type IN ('lampadaire', 'lcu', 'group', 'system', 'cabinet', 'basestation'));
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_work_order_team') THEN
				ALTER TABLE work_orders ADD CONSTRAINT chk_work_order_team
					CHECK (team_type IN ('lighting', 'network', 'electrical', 'inspection'));
			END IF;
		END $$;

		ALTER TABLE interventions ADD COLUMN IF NOT EXISTS work_order_id INT REFERENCES work_orders(id) ON DELETE SET NULL;
		ALTER TABLE interventions ADD COLUMN IF NOT EXISTS technician_name TEXT;
		ALTER TABLE interventions ADD COLUMN IF NOT EXISTS action_taken TEXT;
		ALTER TABLE interventions ADD COLUMN IF NOT EXISTS note TEXT;
		ALTER TABLE interventions ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMP WITH TIME ZONE;

		CREATE INDEX IF NOT EXISTS idx_alerts_work_order ON alerts(work_order_id) WHERE work_order_id IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_work_orders_source_alert ON work_orders(source_alert_id) WHERE source_alert_id IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_work_orders_lcu ON work_orders(lcu_id) WHERE lcu_id IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_work_orders_zone ON work_orders(zone) WHERE zone IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_interventions_work_order ON interventions(work_order_id) WHERE work_order_id IS NOT NULL;

	`)
	if err != nil {
		return err
	}
	return ensureSchemaV3(db)
}

func ensureSchemaV3(db *sql.DB) error {
	_, err := db.Exec(`
		-- Work order technician & lifecycle fields (v3)
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS technician_id INT REFERENCES users(id) ON DELETE SET NULL;
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS assigned_to_name TEXT;
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS accepted_at TIMESTAMP WITH TIME ZONE;
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS started_at TIMESTAMP WITH TIME ZONE;
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMP WITH TIME ZONE;
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS created_by INT REFERENCES users(id) ON DELETE SET NULL;
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS closed_by INT REFERENCES users(id) ON DELETE SET NULL;
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS resolution_type TEXT;
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS closing_note TEXT;

		-- Extend source_type constraint to include 'calculator'
		ALTER TABLE work_orders DROP CONSTRAINT IF EXISTS chk_work_order_source;
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_work_order_source_v2') THEN
				ALTER TABLE work_orders ADD CONSTRAINT chk_work_order_source_v2
					CHECK (source_type IN ('alert', 'manual', 'system', 'calculator'));
			END IF;
		END $$;

		-- Work order logs / audit trail
		CREATE TABLE IF NOT EXISTS work_order_logs (
			id SERIAL PRIMARY KEY,
			work_order_id INT NOT NULL REFERENCES work_orders(id) ON DELETE CASCADE,
			user_id INT REFERENCES users(id) ON DELETE SET NULL,
			user_name TEXT,
			role TEXT,
			action TEXT NOT NULL,
			note TEXT,
			old_status TEXT,
			new_status TEXT,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_wo_logs_work_order ON work_order_logs(work_order_id);
		CREATE INDEX IF NOT EXISTS idx_wo_logs_created ON work_order_logs(created_at DESC);
	`)
	if err != nil {
		return err
	}
	return ensureSchemaV4(db)
}

func ensureSchemaV4(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS audit_logs (
			id SERIAL PRIMARY KEY,
			user_id INTEGER NULL,
			user_name TEXT NULL,
			user_role TEXT NULL,
			action TEXT NOT NULL,
			entity_type TEXT NOT NULL,
			entity_id INTEGER NULL,
			entity_reference TEXT NULL,
			description TEXT NOT NULL,
			old_values JSONB NULL,
			new_values JSONB NULL,
			status TEXT NOT NULL DEFAULT 'success',
			ip_address TEXT NULL,
			user_agent TEXT NULL,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
		CREATE INDEX IF NOT EXISTS idx_audit_logs_entity ON audit_logs(entity_type, entity_id);
		CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id);
		CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_audit_logs_status ON audit_logs(status);
	`)
	if err != nil {
		return err
	}
	return ensureSchemaV5(db)
}

func ensureSchemaV5(db *sql.DB) error {
	_, err := db.Exec(`
		-- Maintenance windows: extend with rich fields
		ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS title TEXT;
		ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS maintenance_type TEXT NOT NULL DEFAULT 'preventive';
		ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS target_type TEXT NOT NULL DEFAULT 'zone';
		ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS target_id INTEGER;
		ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS target_reference TEXT;
		ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS impact_level TEXT NOT NULL DEFAULT 'low';
		ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS suppress_alerts BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS suppress_auto_work_orders BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'planned';
		ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS related_work_order_id INTEGER REFERENCES work_orders(id) ON DELETE SET NULL;
		ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW();
		ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMP WITH TIME ZONE;
		ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS completed_at TIMESTAMP WITH TIME ZONE;
		ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS create_work_order BOOLEAN NOT NULL DEFAULT false;

		-- Rename starts_at/ends_at to start_at/end_at if not already done
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.columns
					   WHERE table_name='maintenance_windows' AND column_name='starts_at')
			   AND NOT EXISTS (SELECT 1 FROM information_schema.columns
					   WHERE table_name='maintenance_windows' AND column_name='start_at') THEN
				ALTER TABLE maintenance_windows RENAME COLUMN starts_at TO start_at;
			END IF;
			IF EXISTS (SELECT 1 FROM information_schema.columns
					   WHERE table_name='maintenance_windows' AND column_name='ends_at')
			   AND NOT EXISTS (SELECT 1 FROM information_schema.columns
					   WHERE table_name='maintenance_windows' AND column_name='end_at') THEN
				ALTER TABLE maintenance_windows RENAME COLUMN ends_at TO end_at;
			END IF;
		END $$;

		-- Alerts: flag maintenance-related alerts
		ALTER TABLE alerts ADD COLUMN IF NOT EXISTS maintenance_related BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE alerts ADD COLUMN IF NOT EXISTS maintenance_window_id INTEGER REFERENCES maintenance_windows(id) ON DELETE SET NULL;

		-- Work orders: link to maintenance window
		ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS maintenance_window_id INTEGER REFERENCES maintenance_windows(id) ON DELETE SET NULL;
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_work_order_source_v3') THEN
				ALTER TABLE work_orders DROP CONSTRAINT IF EXISTS chk_work_order_source_v2;
				ALTER TABLE work_orders ADD CONSTRAINT chk_work_order_source_v3
					CHECK (source_type IN ('alert', 'manual', 'system', 'calculator', 'maintenance_window'));
			END IF;
		END $$;

		CREATE INDEX IF NOT EXISTS idx_maintenance_status ON maintenance_windows(status);
		CREATE INDEX IF NOT EXISTS idx_maintenance_start ON maintenance_windows(start_at);
		CREATE INDEX IF NOT EXISTS idx_maintenance_end ON maintenance_windows(end_at);
		CREATE INDEX IF NOT EXISTS idx_alerts_maintenance ON alerts(maintenance_window_id) WHERE maintenance_window_id IS NOT NULL;
	`)
	return err
}
