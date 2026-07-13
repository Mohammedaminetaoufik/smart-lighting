package services

import (
	"math"
	"time"
)

// SunTimes holds the computed sunrise/sunset for a location and date, plus
// whether it is currently night (lamps should be on).
type SunTimes struct {
	Sunrise  time.Time `json:"sunrise"`
	Sunset   time.Time `json:"sunset"`
	IsNight  bool      `json:"is_night"`
	DaylightH float64  `json:"daylight_hours"`
}

const (
	// Official zenith for sunrise/sunset (accounts for atmospheric refraction).
	sunZenith = 90.833
	deg2rad   = math.Pi / 180.0
	rad2deg   = 180.0 / math.Pi
)

// SunriseSunset computes sunrise and sunset for a given latitude/longitude and
// date, returned in the location `loc` timezone. Implements the standard
// almanac Sunrise/Sunset algorithm (NOAA-equivalent). Handles polar day/night
// by returning zero times, but that never occurs at Moroccan latitudes.
func SunriseSunset(lat, lng float64, date time.Time, loc *time.Location) SunTimes {
	if loc == nil {
		loc = time.UTC
	}
	sunrise, okR := sunEvent(lat, lng, date, true)
	sunset, okS := sunEvent(lat, lng, date, false)

	res := SunTimes{}
	if okR {
		res.Sunrise = sunrise.In(loc)
	}
	if okS {
		res.Sunset = sunset.In(loc)
	}
	if okR && okS {
		res.DaylightH = sunset.Sub(sunrise).Hours()
		now := time.Now()
		res.IsNight = now.Before(sunrise) || now.After(sunset)
	}
	return res
}

// sunEvent computes one event (sunrise if rise=true, else sunset) as a UTC time.
func sunEvent(lat, lng float64, date time.Time, rise bool) (time.Time, bool) {
	year, month, day := date.Date()
	n := dayOfYear(year, int(month), day)

	lngHour := lng / 15.0
	var t float64
	if rise {
		t = float64(n) + ((6 - lngHour) / 24)
	} else {
		t = float64(n) + ((18 - lngHour) / 24)
	}

	// Sun's mean anomaly
	m := (0.9856 * t) - 3.289
	// Sun's true longitude
	l := m + (1.916 * math.Sin(m*deg2rad)) + (0.020 * math.Sin(2*m*deg2rad)) + 282.634
	l = normalizeDeg(l)

	// Sun's right ascension, put in same quadrant as L
	ra := rad2deg * math.Atan(0.91764*math.Tan(l*deg2rad))
	ra = normalizeDeg(ra)
	lQuadrant := math.Floor(l/90) * 90
	raQuadrant := math.Floor(ra/90) * 90
	ra = ra + (lQuadrant - raQuadrant)
	ra = ra / 15 // to hours

	// Sun's declination
	sinDec := 0.39782 * math.Sin(l*deg2rad)
	cosDec := math.Cos(math.Asin(sinDec))

	// Local hour angle
	cosH := (math.Cos(sunZenith*deg2rad) - (sinDec * math.Sin(lat*deg2rad))) / (cosDec * math.Cos(lat*deg2rad))
	if cosH > 1 {
		return time.Time{}, false // sun never rises this day
	}
	if cosH < -1 {
		return time.Time{}, false // sun never sets this day
	}

	var h float64
	if rise {
		h = 360 - rad2deg*math.Acos(cosH)
	} else {
		h = rad2deg * math.Acos(cosH)
	}
	h = h / 15

	// Local mean time of the event
	localT := h + ra - (0.06571 * t) - 6.622
	// To UTC
	ut := normalizeHours(localT - lngHour)

	hours := int(ut)
	minutes := int((ut - float64(hours)) * 60)
	seconds := int((((ut - float64(hours)) * 60) - float64(minutes)) * 60)

	return time.Date(year, month, day, hours, minutes, seconds, 0, time.UTC), true
}

func dayOfYear(year, month, day int) int {
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return t.YearDay()
}

func normalizeDeg(d float64) float64 {
	for d < 0 {
		d += 360
	}
	for d >= 360 {
		d -= 360
	}
	return d
}

func normalizeHours(h float64) float64 {
	for h < 0 {
		h += 24
	}
	for h >= 24 {
		h -= 24
	}
	return h
}
