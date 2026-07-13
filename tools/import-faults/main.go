// Command import-faults backfills fault history from the labeled fault-prediction
// dataset, so the predictive-maintenance features have realistic data.
//
// Usage:
//
//	go run . <chemin_csv> [--no-anchor]
//
// CSV columns: bulb_number,timestamp,power_consumption,voltage_levels,
//              current_fluctuations,temperature,environmental_conditions,
//              current_fluctuations_env,fault_type
//
// Only actual faults (fault_type 1-4) are stored as fault_events. Each dataset
// bulb is mapped onto a real lampadaire (modulo the park). Timestamps are
// re-anchored so the year of data lands on the last 12 months. A lampadaire's
// current fault_status is set from its most recent fault within the last 14
// days (else 'none') — a realistic "currently at risk" set.
package main

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	"map-interactif/internal/repository"
)

const (
	batchRows = 500
	source    = "fault_dataset"
)

// faultMeta maps a fault_type to a short status code and a human label.
var faultMeta = map[int][2]string{
	0: {"none", "Sain"},
	1: {"overcurrent", "Surintensité"},
	2: {"overvoltage", "Surtension"},
	3: {"underpower", "Sous-consommation"},
	4: {"leakage", "Fuite de courant"},
}

type row struct {
	bulb      int
	ts        time.Time
	power     float64
	voltage   float64
	current   float64
	temp      float64
	weather   string
	currEnv   float64
	faultType int
}

func main() {
	var csvPath string
	noAnchor := false
	for _, a := range os.Args[1:] {
		switch {
		case a == "--no-anchor":
			noAnchor = true
		case strings.HasPrefix(a, "--"):
			log.Fatalf("option inconnue: %s", a)
		default:
			csvPath = a
		}
	}
	if csvPath == "" {
		log.Fatal("usage: import-faults [--no-anchor] <chemin_csv>")
	}

	for _, p := range []string{".env", "../../.env", "../../../.env"} {
		if err := godotenv.Load(p); err == nil {
			break
		}
	}

	db, err := repository.OpenDB()
	if err != nil {
		log.Fatalf("connexion DB: %v", err)
	}
	defer db.Close()

	rows, err := readCSV(csvPath)
	if err != nil {
		log.Fatalf("lecture CSV: %v", err)
	}
	log.Printf("CSV: %d lignes lues", len(rows))

	offset := time.Duration(0)
	if !noAnchor && len(rows) > 0 {
		maxTs := rows[0].ts
		for _, r := range rows {
			if r.ts.After(maxTs) {
				maxTs = r.ts
			}
		}
		days := math.Floor(time.Since(maxTs).Hours() / 24)
		offset = time.Duration(days) * 24 * time.Hour
		log.Printf("réancrage: décalage de %.0f jours (dernière panne → maintenant)", days)
	}

	lampIDs, err := loadLampIDs(db)
	if err != nil {
		log.Fatalf("chargement lampadaires: %v", err)
	}
	if len(lampIDs) == 0 {
		log.Fatal("aucun lampadaire actif")
	}
	log.Printf("parc: %d lampadaires", len(lampIDs))

	// Purge idempotente.
	res, _ := db.Exec(`DELETE FROM fault_events WHERE source = $1`, source)
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("purge: %d événements précédents supprimés", n)
	}

	total := insertFaults(db, rows, lampIDs, offset)
	log.Printf("✓ %d événements de panne insérés (fault_type 1-4)", total)

	updateFaultStatus(db)
	log.Printf("✓ fault_status mis à jour : watchlist des ~20%% lampadaires les plus problématiques")
}

func readCSV(path string) ([]row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("CSV vide")
	}

	out := make([]row, 0, len(records)-1)
	for _, rec := range records[1:] {
		if len(rec) < 9 {
			continue
		}
		ts, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(rec[1]), time.Local)
		if err != nil {
			continue
		}
		bulb, _ := strconv.Atoi(strings.TrimSpace(rec[0]))
		ft, _ := strconv.Atoi(strings.TrimSpace(rec[8]))
		out = append(out, row{
			bulb:      bulb,
			ts:        ts,
			power:     parseF(rec[2]),
			voltage:   parseF(rec[3]),
			current:   parseF(rec[4]),
			temp:      parseF(rec[5]),
			weather:   strings.TrimSpace(rec[6]),
			currEnv:   parseF(rec[7]),
			faultType: ft,
		})
	}
	return out, nil
}

func parseF(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

func loadLampIDs(db *sql.DB) ([]int, error) {
	rows, err := db.Query(`SELECT id FROM lampadaires WHERE archived_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func insertFaults(db *sql.DB, rows []row, lampIDs []int, offset time.Duration) int {
	n := len(lampIDs)
	var (
		args  []any
		holds []string
		total int
	)

	flush := func() {
		if len(holds) == 0 {
			return
		}
		q := `INSERT INTO fault_events
			(lampadaire_id, fault_type, label, confidence, puissance, tension, courant, courant_env, temperature, weather, source, created_at)
			VALUES ` + strings.Join(holds, ",")
		if _, err := db.Exec(q, args...); err != nil {
			log.Fatalf("insert batch: %v", err)
		}
		total += len(holds)
		args = args[:0]
		holds = holds[:0]
	}

	for _, r := range rows {
		if r.faultType == 0 {
			continue // on ne stocke que les vraies pannes
		}
		lampID := lampIDs[(r.bulb-1+n)%n] // mappe bulb → lampadaire réel
		meta := faultMeta[r.faultType]
		ts := r.ts.Add(offset)
		i := len(args)
		holds = append(holds, fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			i+1, i+2, i+3, i+4, i+5, i+6, i+7, i+8, i+9, i+10, i+11, i+12))
		args = append(args, lampID, r.faultType, meta[1], 1.0, r.power, r.voltage, r.current, r.currEnv, r.temp, r.weather, source, ts)
		if len(holds) >= batchRows {
			flush()
		}
	}
	flush()
	return total
}

// updateFaultStatus builds a realistic "at-risk" watchlist: the ~20 % most
// fault-prone lampadaires (top quintile by fault count) are flagged with their
// dominant fault type; the rest are reset to healthy ('none'). This mirrors how
// predictive maintenance prioritises chronically failing equipment.
func updateFaultStatus(db *sql.DB) {
	_, _ = db.Exec(`UPDATE lampadaires SET fault_status = 'none' WHERE archived_at IS NULL`)

	_, err := db.Exec(`
		WITH lamp_faults AS (
			SELECT lampadaire_id,
			       COUNT(*) AS n,
			       mode() WITHIN GROUP (ORDER BY fault_type) AS dominant
			FROM fault_events
			GROUP BY lampadaire_id
		),
		ranked AS (
			SELECT lampadaire_id, dominant, ntile(5) OVER (ORDER BY n DESC) AS quintile
			FROM lamp_faults
		)
		UPDATE lampadaires l
		SET fault_status = CASE r.dominant
			WHEN 1 THEN 'overcurrent'
			WHEN 2 THEN 'overvoltage'
			WHEN 3 THEN 'underpower'
			WHEN 4 THEN 'leakage'
			ELSE 'none' END,
		    updated_at = NOW()
		FROM ranked r
		WHERE l.id = r.lampadaire_id AND r.quintile = 1 AND l.archived_at IS NULL`)
	if err != nil {
		log.Printf("Warning: update fault_status: %v", err)
	}
}
