package controllers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"map-interactif/internal/services"
)

// moroccoLoc is UTC+1 (Western European Summer Time is not observed year-round
// consistently, but +1 matches the civil time used for public lighting schedules).
var moroccoLoc = time.FixedZone("Maroc", 1*3600)

// Default coordinates (Casablanca) when none are supplied.
const (
	defaultLat = 33.5731
	defaultLng = -7.5898
)

// HandleGetSunTimes handles GET /api/astronomy/sun?lat=&lng=&date=YYYY-MM-DD.
// Returns sunrise/sunset for the location — the basis for astronomical
// (dusk-to-dawn) switching of public lighting.
func HandleGetSunTimes() gin.HandlerFunc {
	return func(c *gin.Context) {
		lat := defaultLat
		lng := defaultLng
		if v, err := strconv.ParseFloat(c.Query("lat"), 64); err == nil {
			lat = v
		}
		if v, err := strconv.ParseFloat(c.Query("lng"), 64); err == nil {
			lng = v
		}

		date := time.Now().In(moroccoLoc)
		if raw := c.Query("date"); raw != "" {
			if d, err := time.ParseInLocation("2006-01-02", raw, moroccoLoc); err == nil {
				date = d
			}
		}

		sun := services.SunriseSunset(lat, lng, date, moroccoLoc)
		RespondJSON(c, http.StatusOK, gin.H{
			"latitude":       lat,
			"longitude":      lng,
			"date":           date.Format("2006-01-02"),
			"sunrise":        formatOrEmpty(sun.Sunrise),
			"sunset":         formatOrEmpty(sun.Sunset),
			"is_night":       sun.IsNight,
			"daylight_hours": sun.DaylightH,
		})
	}
}

func formatOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
