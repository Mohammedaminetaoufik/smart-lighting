package main

import (
	"database/sql"
	"time"
)

func insertBasestation(db *sql.DB, b *Basestation) error {
	return db.QueryRow(`
		INSERT INTO basestations (reference, name, zone, address, latitude, longitude,
			status, network_type, primary_backhaul, active_backhaul,
			connected_nodes_count, disconnected_nodes_count, signal_quality_avg, battery_status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING id, created_at, updated_at`,
		b.Reference, b.Name, b.Zone, b.Address, b.Latitude, b.Longitude,
		orDefault(b.Status, "unknown"),
		orDefault(b.NetworkType, "Simulated"),
		orDefault(b.PrimaryBackhaul, "simulated"),
		b.ActiveBackhaul,
		b.ConnectedNodesCount, b.DisconnectedNodesCount,
		b.SignalQualityAvg,
		orDefault(b.BatteryStatus, "unknown"),
	).Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
}

const basestationSelectCols = `
	SELECT id, reference, name, zone, address, latitude, longitude,
		status, network_type, primary_backhaul, active_backhaul,
		connected_nodes_count, disconnected_nodes_count, signal_quality_avg, battery_status,
		last_seen_at, commissioned_at, created_at, updated_at
	FROM basestations`

func scanBasestationRow(scan func(...any) error) (Basestation, error) {
	var b Basestation
	var lat, lng sql.NullFloat64
	var lastSeen, commissioned sql.NullTime
	var activeBackhaul sql.NullString
	err := scan(
		&b.ID, &b.Reference, &b.Name, &b.Zone, &b.Address,
		&lat, &lng,
		&b.Status, &b.NetworkType, &b.PrimaryBackhaul, &activeBackhaul,
		&b.ConnectedNodesCount, &b.DisconnectedNodesCount,
		&b.SignalQualityAvg, &b.BatteryStatus,
		&lastSeen, &commissioned,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return b, err
	}
	if lat.Valid {
		b.Latitude = &lat.Float64
	}
	if lng.Valid {
		b.Longitude = &lng.Float64
	}
	if lastSeen.Valid {
		b.LastSeenAt = &lastSeen.Time
	}
	if commissioned.Valid {
		b.CommissionedAt = &commissioned.Time
	}
	if activeBackhaul.Valid {
		b.ActiveBackhaul = activeBackhaul.String
	}
	return b, nil
}

func listBasestations(db *sql.DB) ([]Basestation, error) {
	rows, err := db.Query(basestationSelectCols + ` ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Basestation
	for rows.Next() {
		if b, err := scanBasestationRow(rows.Scan); err == nil {
			list = append(list, b)
		}
	}
	return list, nil
}

func getBasestationByID(db *sql.DB, id int) (*Basestation, error) {
	row := db.QueryRow(basestationSelectCols+` WHERE id = $1`, id)
	b, err := scanBasestationRow(row.Scan)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func updateBasestationStatus(db *sql.DB, id int, status string) error {
	now := time.Now()
	_, err := db.Exec(`UPDATE basestations SET status=$1, last_seen_at=$2, updated_at=$2 WHERE id=$3`,
		status, now, id)
	return err
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
