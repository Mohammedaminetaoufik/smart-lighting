package main

import (
	"context"
	"database/sql"
)

func getDashboardStats(ctx context.Context, db *sql.DB) (*DashboardStats, error) {
	stats := &DashboardStats{}

	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lcus").Scan(&stats.TotalLCUs)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lcus WHERE status='online'").Scan(&stats.LCUsOnline)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lcus WHERE status='offline'").Scan(&stats.LCUsOffline)

	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lampadaires WHERE archived_at IS NULL").Scan(&stats.TotalLampadaires)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lampadaires WHERE archived_at IS NULL AND etat='online'").Scan(&stats.LampadairesOnline)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lampadaires WHERE archived_at IS NULL AND etat='offline'").Scan(&stats.LampadairesOffline)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lampadaires WHERE archived_at IS NULL AND etat='maintenance'").Scan(&stats.LampadairesMaintenance)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lampadaires WHERE archived_at IS NULL AND (latitude=0 OR latitude IS NULL) AND location_status='missing'").Scan(&stats.MissingLocation)

	db.QueryRowContext(ctx, "SELECT COALESCE(AVG(intensite),0) FROM lampadaires WHERE archived_at IS NULL").Scan(&stats.AvgIntensity)
	db.QueryRowContext(ctx, "SELECT COALESCE(AVG(sm.puissance),0) FROM sensor_measurements sm INNER JOIN (SELECT DISTINCT ON (lampadaire_id) id FROM sensor_measurements ORDER BY lampadaire_id, created_at DESC) latest ON sm.id = latest.id").Scan(&stats.AvgPower)

	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM alerts WHERE status='open'").Scan(&stats.OpenAlerts)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM alerts WHERE status='open' AND severity='critical'").Scan(&stats.CriticalAlerts)

	db.QueryRowContext(ctx, "SELECT COALESCE(SUM(puissance - (puissance * intensite / 100.0)), 0), COALESCE(AVG(100 - intensite), 0) FROM lampadaires WHERE archived_at IS NULL AND puissance IS NOT NULL").Scan(&stats.EstimatedPowerSavingW, &stats.EstimatedSavingPercent)

	alertRows, err := db.QueryContext(ctx, "SELECT a.id, a.lampadaire_id, a.type, a.severity, a.message, a.status, a.created_at, a.resolved_at, COALESCE(l.reference,'') as reference FROM alerts a LEFT JOIN lampadaires l ON a.lampadaire_id = l.id ORDER BY a.created_at DESC LIMIT 5")
	if err == nil {
		defer alertRows.Close()
		for alertRows.Next() {
			var al Alert
			var lid sql.NullInt64
			var resolved sql.NullTime
			if err := alertRows.Scan(&al.ID, &lid, &al.Type, &al.Severity, &al.Message, &al.Status, &al.CreatedAt, &resolved, &al.Reference); err == nil {
				if lid.Valid {
					v := int(lid.Int64)
					al.LampadaireID = &v
				}
				if resolved.Valid {
					al.ResolvedAt = &resolved.Time
				}
				stats.RecentAlerts = append(stats.RecentAlerts, al)
			}
		}
	}

	cmdRows, err := db.QueryContext(ctx, "SELECT id, lampadaire_id, source, old_intensity, new_intensity, reason, status, created_at, applied_at FROM dimming_commands ORDER BY created_at DESC LIMIT 5")
	if err == nil {
		defer cmdRows.Close()
		for cmdRows.Next() {
			var cmd DimmingCommand
			var oldInt sql.NullInt64
			var applied sql.NullTime
			var reason sql.NullString
			if err := cmdRows.Scan(&cmd.ID, &cmd.LampadaireID, &cmd.Source, &oldInt, &cmd.NewIntensity, &reason, &cmd.Status, &cmd.CreatedAt, &applied); err == nil {
				if oldInt.Valid {
					v := int(oldInt.Int64)
					cmd.OldIntensity = &v
				}
				if applied.Valid {
					cmd.AppliedAt = &applied.Time
				}
				if reason.Valid {
					cmd.Reason = reason.String
				}
				stats.RecentCommands = append(stats.RecentCommands, cmd)
			}
		}
	}

	telRows, err := db.QueryContext(ctx, "SELECT id, lampadaire_id, luminosite, presence, temperature, humidite, tension, courant, puissance, energie, source, created_at FROM sensor_measurements ORDER BY created_at DESC LIMIT 5")
	if err == nil {
		defer telRows.Close()
		for telRows.Next() {
			var m SensorMeasurement
			if err := telRows.Scan(&m.ID, &m.LampadaireID, &m.Luminosite, &m.Presence, &m.Temperature, &m.Humidite, &m.Tension, &m.Courant, &m.Puissance, &m.Energie, &m.Source, &m.CreatedAt); err == nil {
				stats.RecentTelemetry = append(stats.RecentTelemetry, m)
			}
		}
	}

	return stats, nil
}
