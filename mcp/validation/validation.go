// Package validation contains the shared, bounded input rules for MCP tools.
package validation

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

const (
	MaxEntityIDLength  = 256
	MaxSearchLength    = 200
	MaxRadiusMeters    = 50_000
	MaxWindowMinutes   = 24 * 60
	MaxTimestampMillis = int64(4_102_444_800_000) // 2100-01-01 UTC
)

var entityIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)

func EntityID(name, value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > MaxEntityIDLength || value == "." || value == ".." || !entityIDPattern.MatchString(value) {
		return "", fmt.Errorf("%s must be 1-%d characters containing only letters, numbers, _, -, ., or :", name, MaxEntityIDLength)
	}
	return value, nil
}

func Latitude(value float64) (float64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < -90 || value > 90 {
		return 0, fmt.Errorf("lat must be a finite number between -90 and 90")
	}
	return value, nil
}

func Longitude(value float64) (float64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < -180 || value > 180 {
		return 0, fmt.Errorf("lon must be a finite number between -180 and 180")
	}
	return value, nil
}

func RadiusMeters(value float64, fallback, maximum int) (int, error) {
	if value == 0 {
		return fallback, nil
	}
	if maximum < 1 || maximum > MaxRadiusMeters {
		maximum = MaxRadiusMeters
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 1 || value > float64(maximum) || value != math.Trunc(value) {
		return 0, fmt.Errorf("radius must be a whole number between 1 and %d meters", maximum)
	}
	return int(value), nil
}

func WholeNumber(name string, value float64, fallback, minimum, maximum int) (int, error) {
	if value == 0 {
		return fallback, nil
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value < float64(minimum) || value > float64(maximum) || value != math.Trunc(value) {
		return 0, fmt.Errorf("%s must be a whole number between %d and %d", name, minimum, maximum)
	}
	return int(value), nil
}

func WindowMinutes(name string, value float64, fallback, maximum int) (int, error) {
	if maximum < 1 || maximum > MaxWindowMinutes {
		maximum = MaxWindowMinutes
	}
	return WholeNumber(name, value, fallback, 0, maximum)
}

func Span(name string, value float64) (float64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > 180 {
		return 0, fmt.Errorf("%s must be a finite number greater than 0 and at most 180", name)
	}
	return value, nil
}

func TimestampMillis(name string, value float64) (int64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 1 || value > float64(MaxTimestampMillis) || value != math.Trunc(value) {
		return 0, fmt.Errorf("%s must be a whole Unix timestamp in milliseconds between 1 and %d", name, MaxTimestampMillis)
	}
	return int64(value), nil
}

func Search(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > MaxSearchLength {
		return "", fmt.Errorf("query must contain 1-%d non-whitespace characters", MaxSearchLength)
	}
	return value, nil
}

func Date(value string) (string, error) {
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return "", fmt.Errorf("date must use YYYY-MM-DD")
	}
	return value, nil
}
