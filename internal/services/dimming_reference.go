package services

import (
	"context"
	"database/sql"
	"sync"
)

// dimmingCondKey identifies a dimming condition WITHOUT weather — the live
// telemetry has no weather channel, so the calculator aggregates over weather.
type dimmingCondKey struct {
	hour     int
	bucket   string // nuit | crepuscule | jour
	presence bool
}

var (
	dimmingRefMu     sync.RWMutex
	dimmingRef       map[dimmingCondKey]float64
	dimmingRefLoaded bool
)

// LoadDimmingReference loads the learned dimming reference table into memory,
// aggregating over weather (sample-count weighted). Safe to call repeatedly.
func LoadDimmingReference(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT hour, lux_bucket, presence, recommended_brightness, sample_count
		FROM dimming_reference`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type acc struct {
		weightedSum float64
		weight      float64
	}
	tmp := map[dimmingCondKey]*acc{}
	for rows.Next() {
		var (
			hour       int
			bucket     string
			presence   bool
			brightness float64
			count      int
		)
		if err := rows.Scan(&hour, &bucket, &presence, &brightness, &count); err != nil {
			continue
		}
		k := dimmingCondKey{hour, bucket, presence}
		a := tmp[k]
		if a == nil {
			a = &acc{}
			tmp[k] = a
		}
		a.weightedSum += brightness * float64(count)
		a.weight += float64(count)
	}

	ref := make(map[dimmingCondKey]float64, len(tmp))
	for k, a := range tmp {
		if a.weight > 0 {
			ref[k] = a.weightedSum / a.weight
		}
	}

	dimmingRefMu.Lock()
	dimmingRef = ref
	dimmingRefLoaded = len(ref) > 0
	dimmingRefMu.Unlock()
	return nil
}

// luxBucketFor classifies ambient luminosity (same thresholds as the import tool).
func luxBucketFor(lux float64) string {
	switch {
	case lux < 10:
		return "nuit"
	case lux < 100:
		return "crepuscule"
	default:
		return "jour"
	}
}

// LookupRecommendedBrightness returns the data-driven recommended intensity for
// the given conditions, and whether a reference exists. Falls back to the
// no-presence profile of the same hour/bucket if the exact key is missing.
func LookupRecommendedBrightness(hour int, lux float64, presence bool) (int, bool) {
	dimmingRefMu.RLock()
	defer dimmingRefMu.RUnlock()
	if !dimmingRefLoaded {
		return 0, false
	}
	bucket := luxBucketFor(lux)
	if v, ok := dimmingRef[dimmingCondKey{hour, bucket, presence}]; ok {
		return int(v + 0.5), true
	}
	// Fallback : même heure/régime, sans présence.
	if v, ok := dimmingRef[dimmingCondKey{hour, bucket, false}]; ok {
		return int(v + 0.5), true
	}
	return 0, false
}

// DimmingReferenceReady reports whether a learned reference is loaded.
func DimmingReferenceReady() bool {
	dimmingRefMu.RLock()
	defer dimmingRefMu.RUnlock()
	return dimmingRefLoaded
}
