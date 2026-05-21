package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"map-interactif/internal/repository"
)

// ActiveWindow holds the fields from a maintenance window relevant to suppression logic.
type ActiveWindow struct {
	ID                     int
	Title                  string
	MaintenanceType        string
	TargetType             string
	SuppressAlerts         bool
	SuppressAutoWorkOrders bool
	ImpactLevel            string
}

// IsEquipmentInMaintenance checks whether a lampadaire is covered by any currently
// active maintenance window (start_at <= now <= end_at, status in planned/active).
func IsEquipmentInMaintenance(ctx context.Context, db repository.DBExecutor, lampadaireID int, zone string, lcuID *int) (bool, *ActiveWindow) {
	now := time.Now()
	rows, err := db.QueryContext(ctx, `
		SELECT id, COALESCE(title,''), COALESCE(maintenance_type,'preventive'),
		       COALESCE(target_type,'zone'), target_id, COALESCE(zone,''),
		       lampadaire_ids, suppress_alerts, suppress_auto_work_orders,
		       COALESCE(impact_level,'low')
		FROM maintenance_windows
		WHERE status IN ('planned','active')
		  AND start_at <= $1 AND end_at >= $1
	`, now)
	if err != nil {
		return false, nil
	}
	defer rows.Close()

	for rows.Next() {
		var w ActiveWindow
		var targetID sql.NullInt64
		var wZone string
		var lampadaireIDsRaw sql.NullString

		if err := rows.Scan(
			&w.ID, &w.Title, &w.MaintenanceType, &w.TargetType,
			&targetID, &wZone, &lampadaireIDsRaw,
			&w.SuppressAlerts, &w.SuppressAutoWorkOrders, &w.ImpactLevel,
		); err != nil {
			continue
		}

		switch w.TargetType {
		case "global":
			return true, &w
		case "zone":
			if wZone != "" && wZone == zone {
				return true, &w
			}
		case "lampadaire":
			if targetID.Valid && int(targetID.Int64) == lampadaireID {
				return true, &w
			}
		case "lcu":
			if targetID.Valid && lcuID != nil && int(targetID.Int64) == *lcuID {
				return true, &w
			}
		case "selection":
			if lampadaireIDsRaw.Valid && lampadaireIDsRaw.String != "" {
				var ids []int
				if err := json.Unmarshal([]byte(lampadaireIDsRaw.String), &ids); err == nil {
					for _, id := range ids {
						if id == lampadaireID {
							return true, &w
						}
					}
				}
			}
		}
	}
	return false, nil
}

// MarkAlertMaintenanceRelated flags an alert as maintenance-related and links it to the window.
func MarkAlertMaintenanceRelated(ctx context.Context, db *sql.DB, alertID, windowID int) {
	db.ExecContext(ctx,
		`UPDATE alerts SET maintenance_related=true, maintenance_window_id=$1 WHERE id=$2`,
		windowID, alertID)
}
