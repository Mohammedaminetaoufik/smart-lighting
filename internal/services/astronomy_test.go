package services

import (
	"testing"
	"time"
)

func TestSunriseSunsetCasablanca(t *testing.T) {
	// Casablanca, Maroc
	lat, lng := 33.5731, -7.5898
	loc := time.FixedZone("WEST", 1*3600) // UTC+1

	summer := time.Date(2024, 6, 21, 12, 0, 0, 0, loc) // solstice d'été
	winter := time.Date(2024, 12, 21, 12, 0, 0, 0, loc) // solstice d'hiver

	s := SunriseSunset(lat, lng, summer, loc)
	if s.Sunrise.IsZero() || s.Sunset.IsZero() {
		t.Fatal("été : lever/coucher non calculés")
	}
	// En été à Casablanca : lever ~6h, coucher ~19h30-20h (heure locale)
	if h := s.Sunrise.Hour(); h < 5 || h > 7 {
		t.Errorf("été : lever attendu 5-7h, obtenu %dh", h)
	}
	if h := s.Sunset.Hour(); h < 19 || h > 21 {
		t.Errorf("été : coucher attendu 19-21h, obtenu %dh", h)
	}
	if s.DaylightH < 13 {
		t.Errorf("été : durée du jour attendue > 13h, obtenue %.1fh", s.DaylightH)
	}

	w := SunriseSunset(lat, lng, winter, loc)
	if w.DaylightH >= s.DaylightH {
		t.Errorf("le jour d'hiver (%.1fh) doit être plus court que l'été (%.1fh)", w.DaylightH, s.DaylightH)
	}
	if w.DaylightH > 11 {
		t.Errorf("hiver : durée du jour attendue < 11h, obtenue %.1fh", w.DaylightH)
	}
}
