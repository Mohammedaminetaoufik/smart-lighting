package utils

import (
	"fmt"
	"strconv"
	"time"
)

// PtrFloat64 returns a pointer to a float64 value.
func PtrFloat64(v float64) *float64 {
	return &v
}

// PtrBool returns a pointer to a bool value.
func PtrBool(v bool) *bool {
	return &v
}

// PtrInt returns a pointer to an int value.
func PtrInt(v int) *int {
	return &v
}

// ParseOptionalInt parses an optional integer string.
func ParseOptionalInt(value string) (*int, error) {
	if value == "" {
		return nil, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return nil, fmt.Errorf("invalid")
	}

	return &parsed, nil
}

// ParseOptionalDate parses an optional date string in YYYY-MM-DD format.
func ParseOptionalDate(value string) (*string, error) {
	if value == "" {
		return nil, nil
	}

	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, err
	}

	formatted := parsed.Format("2006-01-02")
	return &formatted, nil
}

// NullableInt returns nil for nil pointer, or the dereferenced int as interface{}.
func NullableInt(v *int) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

// OrDefault returns s if non-empty, otherwise def.
func OrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// NullString returns nil for empty strings, or the string value.
func NullString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

// ClampIntensity ensures intensity is between 0 and 100.
func ClampIntensity(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// MinInt returns the smaller of two integers.
func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
