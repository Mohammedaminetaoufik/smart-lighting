package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"time"
)

// SeedMockDataIfEmpty checks if tables are empty and seeds them with beautiful initial data.
func SeedMockDataIfEmpty(db *sql.DB) error {
	ctx := context.Background()

	// 1. Seed default user if empty
	var userCount int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		return err
	}
	if userCount == 0 {
		log.Println("Seeding default admin user...")
		_, err = db.ExecContext(ctx, `
			INSERT INTO users (full_name, email, password_hash, role, status)
			VALUES ('Admin Lamalif', 'admin@lamalif.ma', '$2a$10$wB5V0E/l0jK1oFfS7MhBie3Koxk25YFqI0kR.qU8r5Tqf5v5q9G6G', 'admin', 'active')
		`)
		if err != nil {
			return fmt.Errorf("error seeding user: %w", err)
		}
	}

	// 2. Seed lampadaires if empty
	var lampCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lampadaires WHERE archived_at IS NULL").Scan(&lampCount)
	if err != nil {
		return err
	}

	if lampCount == 0 {
		log.Println("Seeding mock lampadaires...")
		zones := []string{"Zone Nord", "Zone Centre", "Zone Sud", "Zone Est"}
		types := []string{"LED-150W", "LED-250W", "SHP-400W"}
		powers := []int{150, 250, 400}

		// Coordinates around Casablanca
		baseLat := 33.5731
		baseLng := -7.5898

		for i := 1; i <= 15; i++ {
			ref := fmt.Sprintf("LMP-%03d", i)
			zone := zones[rand.Intn(len(zones))]
			typeDriver := types[rand.Intn(len(types))]
			power := powers[rand.Intn(len(powers))]

			// Slight offset for coordinates
			lat := baseLat + (rand.Float64()-0.5)*0.01
			lng := baseLng + (rand.Float64()-0.5)*0.01

			// Alternate status
			status := "online"
			if i == 5 {
				status = "maintenance"
			} else if i == 11 {
				status = "offline"
			}

			_, err = db.ExecContext(ctx, `
				INSERT INTO lampadaires (reference, latitude, longitude, zone, type_driver, puissance, etat, intensite, date_installation)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CURRENT_DATE - INTERVAL '1 year')
			`, ref, lat, lng, zone, typeDriver, power, status, 80)
			if err != nil {
				return fmt.Errorf("error seeding lampadaire %d: %w", i, err)
			}
		}
	}

	// 3. Seed sensor measurements if empty or low (indicating outdated or manual test data)
	var measCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sensor_measurements").Scan(&measCount)
	if err != nil {
		return err
	}

	if measCount < 1000 {
		log.Println("Database has low measurement count. Truncating and seeding 30 days of historical energy telemetry...")
		_, _ = db.ExecContext(ctx, "TRUNCATE TABLE sensor_measurements RESTART IDENTITY")

		// Get all lampadaire IDs and powers
		rows, err := db.QueryContext(ctx, "SELECT id, puissance FROM lampadaires WHERE archived_at IS NULL")
		if err != nil {
			return err
		}
		defer rows.Close()

		type Lamp struct {
			ID        int
			Puissance int
		}
		var lamps []Lamp
		for rows.Next() {
			var l Lamp
			if err := rows.Scan(&l.ID, &l.Puissance); err == nil {
				lamps = append(lamps, l)
			}
		}

		if len(lamps) == 0 {
			return nil
		}

		// Use a transaction for fast inserts
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO sensor_measurements
			(lampadaire_id, luminosite, presence, temperature, humidite, tension, courant, puissance, energie, source, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		now := time.Now()

		for _, lamp := range lamps {
			// For each of the last 30 days
			for d := 29; d >= 0; d-- {
				dayDate := now.AddDate(0, 0, -d)

				// Seed 6 measurements per day (every 4 hours: 00:00, 04:00, 08:00, 12:00, 16:00, 20:00)
				for h := 0; h < 24; h += 4 {
					measTime := time.Date(dayDate.Year(), dayDate.Month(), dayDate.Day(), h, 0, 0, 0, time.Local)
					if measTime.After(now) {
						continue
					}

					// Default day/night behaviors
					isNight := h <= 4 || h >= 20
					var currentPower float64
					var energyKWh float64
					var brightness float64
					presence := rand.Float64() > 0.7

					if isNight {
						// Lamp is ON at 80% intensity
						currentPower = float64(lamp.Puissance) * 0.8
						// Spanned over 4 hours: power * hours / 1000
						energyKWh = (currentPower * 4.0) / 1000.0
						// Add some random variance
						energyKWh += (rand.Float64() - 0.5) * 0.05 * energyKWh
						if energyKWh < 0 {
							energyKWh = 0
						}
						brightness = 5.0 + rand.Float64()*10.0
					} else {
						// Lamp is OFF
						currentPower = 0.0
						energyKWh = 0.0
						brightness = 200.0 + rand.Float64()*800.0
					}

					temp := 15.0 + rand.Float64()*15.0
					humidity := 40.0 + rand.Float64()*40.0
					voltage := 220.0 + (rand.Float64()-0.5)*10.0
					current := 0.0
					if currentPower > 0 {
						current = currentPower / voltage
					}

					_, err := stmt.ExecContext(ctx,
						lamp.ID,
						brightness,
						presence,
						temp,
						humidity,
						voltage,
						current,
						currentPower,
						energyKWh,
						"simulation_historical",
						measTime,
					)
					if err != nil {
						return fmt.Errorf("failed to insert measurement: %w", err)
					}
				}
			}
		}

		if err := tx.Commit(); err != nil {
			return err
		}
		log.Println("Seeding successfully completed!")
	}

	return nil
}
