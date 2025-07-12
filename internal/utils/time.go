package utils

import (
	"fmt"
	"time"
)

const (
	// UniversalFormat defines the YYYY-MM-DD-HH pattern used consistently across:
	// - Recording filenames (2024-01-15-14.mp3)
	// - API endpoints (/recordings/2024-01-15-14)
	// - Cache keys and internal timestamps
	// This format ensures proper sorting and hour-boundary alignment
	UniversalFormat = "2006-01-02-15"

	// DisplayFormat provides human-readable timestamps for API responses
	// Converts "2024-01-15-14" to "15-01-2024 14:00" for user interfaces
	DisplayFormat = "02-01-2006 15:04"
)

// GetAppTimezone returns the application's standard timezone
// Falls back to UTC if timezone loading fails
func GetAppTimezone(timezone string) *time.Location {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func NowInTimezone(timezone string) time.Time {
	return time.Now().In(GetAppTimezone(timezone))
}

func FormatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	milliseconds := int(d.Milliseconds()) % 1000

	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, milliseconds)
}

func FormatTimestamp(t time.Time, timezone string) string {
	return t.In(GetAppTimezone(timezone)).Format(UniversalFormat)
}

func ParseTimestamp(timestamp string, timezone string) (time.Time, error) {
	t, err := time.Parse(UniversalFormat, timestamp)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, GetAppTimezone(timezone)), nil
}

func ToAPIString(t time.Time, timezone string) string {
	return t.In(GetAppTimezone(timezone)).Format("2006-01-02 15:04")
}

// GetCurrentHour returns the current hour in UniversalFormat (YYYY-MM-DD-HH)
// Truncates to hour boundary (sets minutes/seconds to 0) for consistent recording naming
// Example: if current time is 14:37:23, returns "2024-01-15-14"
func GetCurrentHour(timezone string) string {
	now := NowInTimezone(timezone)
	return FormatTimestamp(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, GetAppTimezone(timezone)), timezone)
}
