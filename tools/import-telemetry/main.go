// Command import-telemetry backfills the sensor_measurements table from a
// streetlights dataset CSV, so the energy/finance/audit features have realistic
// data to work with.
//
// Usage:
//
//	go run . <chemin_csv> [--purge] [--no-anchor]
//
// The CSV columns are: timestamp,hour,ambient_lux,motion_count,weather,brightness,on_off
// Each CSV row is replicated across every active lampadaire (with a small
// per-lamp variation), and timestamps are re-anchored so the dataset lands on
// the last N days (visible by the "30 days" views and the ONEE bill).
package main

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	"map-interactif/internal/repository"
)

const (
	sampleIntervalMin = 10.0 // le dataset échantillonne toutes les 10 minutes
	batchRows         = 500  // lignes par INSERT multi-values (11 params/ligne)
	source            = "dataset_import"
	defaultNominalW   = 100
)

type csvRow struct {
	ts         time.Time
	ambientLux float64
	motion     int
	weather    string
	brightness float64
	onOff      bool
}

type lamp struct {
	id        int
	nominalW  int
	variation float64 // facteur stable par lampadaire (0.85–1.15)
	etat      string  // état assigné (mix réaliste : online/offline/maintenance)
}

func main() {
	// Parsing manuel : robuste à l'ordre des arguments (le package flag ignore
	// les flags placés après un argument positionnel).
	var csvPath string
	noAnchor := false
	for _, a := range os.Args[1:] {
		switch {
		case a == "--no-anchor":
			noAnchor = true
		case a == "--purge":
			// no-op : la purge est désormais systématique (idempotence).
		case strings.HasPrefix(a, "--"):
			log.Fatalf("option inconnue: %s", a)
		default:
			csvPath = a
		}
	}
	if csvPath == "" {
		log.Fatal("usage: import-telemetry [--no-anchor] <chemin_csv>")
	}

	// Charger le .env du backend (chemins candidats selon le CWD).
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
		// Décaler d'un nombre ENTIER de jours pour préserver l'heure de la
		// journée (sinon la logique jour/nuit du dataset est décalée et la
		// courbe de charge s'inverse).
		days := math.Floor(time.Since(maxTs).Hours() / 24)
		offset = time.Duration(days) * 24 * time.Hour
		log.Printf("réancrage: décalage de %.0f jours entiers (heure de la journée préservée)", days)
	}

	lamps, err := loadLamps(db)
	if err != nil {
		log.Fatalf("chargement lampadaires: %v", err)
	}
	if len(lamps) == 0 {
		log.Fatal("aucun lampadaire actif — importer d'abord des lampadaires")
	}
	log.Printf("parc: %d lampadaires actifs", len(lamps))

	// Purge systématique des imports précédents → relancer l'outil ne crée
	// jamais de doublons.
	res, err := db.Exec(`DELETE FROM sensor_measurements WHERE source = $1`, source)
	if err != nil {
		log.Fatalf("purge: %v", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("purge: %d mesures dataset_import précédentes supprimées", n)
	}

	total := insertMeasurements(db, rows, lamps, offset)
	log.Printf("✓ %d mesures insérées (%d lampadaires × %d lignes)", total, len(lamps), len(rows))

	updateLamps(db, lamps, rows, offset)
	log.Printf("✓ lampadaires mis à jour (energy_kwh, operating_hours, intensité, last_seen)")

	nRef := buildDimmingReference(db, rows)
	log.Printf("✓ dimming_reference: %d profils appris du dataset (conditions → intensité)", nRef)
	log.Println("Note: puissance/énergie/tension sont dérivées (estimation, dimming linéaire).")
}

// luxBucket classe la luminosité ambiante en 3 régimes.
func luxBucket(lux float64) string {
	switch {
	case lux < 10:
		return "nuit"
	case lux < 100:
		return "crepuscule"
	default:
		return "jour"
	}
}

// buildDimmingReference apprend du CSV le brightness moyen par condition
// (heure, régime de luminosité, présence, météo) et le stocke pour piloter le
// calculateur data-driven.
func buildDimmingReference(db *sql.DB, rows []csvRow) int {
	// Table créée par le backend (ensureSchemaDimmingRef) ; garantie ici pour
	// permettre un import avant le premier démarrage du backend.
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS dimming_reference (
			hour INTEGER NOT NULL CHECK (hour BETWEEN 0 AND 23),
			lux_bucket TEXT NOT NULL, presence BOOLEAN NOT NULL, weather TEXT NOT NULL,
			recommended_brightness DOUBLE PRECISION NOT NULL, sample_count INTEGER NOT NULL DEFAULT 0,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			PRIMARY KEY (hour, lux_bucket, presence, weather)
		)`)

	type key struct {
		hour     int
		bucket   string
		presence bool
		weather  string
	}
	type agg struct {
		sum   float64
		count int
	}
	buckets := map[key]*agg{}
	for _, r := range rows {
		k := key{r.ts.Hour(), luxBucket(r.ambientLux), r.motion > 0, r.weather}
		a := buckets[k]
		if a == nil {
			a = &agg{}
			buckets[k] = a
		}
		a.sum += r.brightness
		a.count++
	}

	_, _ = db.Exec(`TRUNCATE dimming_reference`)
	n := 0
	for k, a := range buckets {
		avg := a.sum / float64(a.count)
		if _, err := db.Exec(`
			INSERT INTO dimming_reference (hour, lux_bucket, presence, weather, recommended_brightness, sample_count)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			k.hour, k.bucket, k.presence, k.weather, round1(avg), a.count); err == nil {
			n++
		}
	}
	return n
}

func readCSV(path string) ([]csvRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("CSV vide")
	}

	out := make([]csvRow, 0, len(records)-1)
	for _, rec := range records[1:] { // skip header
		if len(rec) < 7 {
			continue
		}
		ts, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(rec[0]), time.Local)
		if err != nil {
			continue
		}
		lux, _ := strconv.ParseFloat(rec[2], 64)
		motion, _ := strconv.Atoi(rec[3])
		brightness, _ := strconv.ParseFloat(rec[5], 64)
		onOff := strings.TrimSpace(rec[6]) == "1"
		out = append(out, csvRow{
			ts: ts, ambientLux: lux, motion: motion,
			weather: strings.TrimSpace(rec[4]), brightness: brightness, onOff: onOff,
		})
	}
	return out, nil
}

func loadLamps(db *sql.DB) ([]lamp, error) {
	rows, err := db.Query(`SELECT id, COALESCE(nominal_power_w, puissance, 0) FROM lampadaires WHERE archived_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rng := rand.New(rand.NewSource(42)) // déterministe : mêmes variations à chaque run
	var lamps []lamp
	for rows.Next() {
		var l lamp
		if err := rows.Scan(&l.id, &l.nominalW); err != nil {
			continue
		}
		if l.nominalW <= 0 {
			l.nominalW = defaultNominalW
		}
		l.variation = 0.85 + rng.Float64()*0.30
		// Mix d'états réaliste : ~70 % en ligne, ~15 % hors ligne, ~15 % maintenance.
		switch r := rng.Float64(); {
		case r < 0.70:
			l.etat = "online"
		case r < 0.85:
			l.etat = "offline"
		default:
			l.etat = "maintenance"
		}
		lamps = append(lamps, l)
	}
	return lamps, nil
}

// derive computes a sensor measurement for a lamp from a CSV row.
func derive(r csvRow, l lamp, rng *rand.Rand) (lux float64, presence bool, tempC, humid, tension, courant, puissance, energie float64) {
	lux = r.ambientLux
	presence = r.motion > 0

	brightnessEff := math.Min(100, math.Max(0, r.brightness*l.variation))
	if !r.onOff {
		brightnessEff = 0
	}
	puissance = float64(l.nominalW) * brightnessEff / 100.0
	energie = puissance / 1000.0 * (sampleIntervalMin / 60.0) // kWh sur l'intervalle
	tension = 220 + rng.Float64()*10
	if puissance > 0 {
		courant = puissance / tension
	}
	// charge thermique : ~25°C au repos, +30°C à pleine charge
	tempC = 25 + (brightnessEff/100.0)*30 + rng.Float64()*3
	humid = 45 + rng.Float64()*15
	if r.weather == "rainy" || r.weather == "foggy" {
		humid = 80 + rng.Float64()*15
	}
	return
}

func insertMeasurements(db *sql.DB, rows []csvRow, lamps []lamp, offset time.Duration) int {
	rng := rand.New(rand.NewSource(7))
	const cols = 11
	var (
		args  []any
		holds []string
		total int
	)

	flush := func() {
		if len(holds) == 0 {
			return
		}
		q := `INSERT INTO sensor_measurements
			(lampadaire_id, luminosite, presence, temperature, humidite, tension, courant, puissance, energie, source, created_at)
			VALUES ` + strings.Join(holds, ",")
		if _, err := db.Exec(q, args...); err != nil {
			log.Fatalf("insert batch: %v", err)
		}
		total += len(holds)
		args = args[:0]
		holds = holds[:0]
	}

	for _, r := range rows {
		ts := r.ts.Add(offset)
		for _, l := range lamps {
			lux, presence, tempC, humid, tension, courant, puissance, energie := derive(r, l, rng)
			n := len(args)
			holds = append(holds, fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
				n+1, n+2, n+3, n+4, n+5, n+6, n+7, n+8, n+9, n+10, n+11))
			args = append(args, l.id, lux, presence, tempC, humid, tension, courant, puissance, energie, source, ts)
			if len(holds) >= batchRows {
				flush()
			}
		}
	}
	flush()
	return total
}

// updateLamps refreshes cumulative and current fields per lamp from the dataset.
func updateLamps(db *sql.DB, lamps []lamp, rows []csvRow, offset time.Duration) {
	rng := rand.New(rand.NewSource(11))
	for _, l := range lamps {
		var totalKWh, litSamples, lastBrightness float64
		for _, r := range rows {
			_, _, _, _, _, _, p, e := derive(r, l, rng)
			totalKWh += e
			if p > 0 {
				litSamples++
				lastBrightness = math.Min(100, r.brightness*l.variation)
			}
		}
		operatingHours := litSamples * (sampleIntervalMin / 60.0)
		// last_seen frais pour les lampadaires en ligne, ancien pour les autres
		// (cohérent avec l'état et le job de détection hors-ligne).
		lastSeen := "NOW()"
		if l.etat != "online" {
			lastSeen = "NOW() - INTERVAL '3 hours'"
		}
		_, _ = db.Exec(`
			UPDATE lampadaires
			SET energy_kwh = $1, operating_hours = $2, intensite = $3,
			    etat = $4, last_seen_at = `+lastSeen+`, updated_at = NOW()
			WHERE id = $5`,
			round2(totalKWh), round1(operatingHours), int(lastBrightness), l.etat, l.id)
	}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round2(v float64) float64 { return math.Round(v*100) / 100 }
