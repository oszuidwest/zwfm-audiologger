// Package utils provides time handling utilities with consistent timezone support
// and universal timestamp formatting for the audio logger application.
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
)

// GetAppTimezone returns the application's standard timezone.
// Falls back to UTC if timezone loading fails.
func GetAppTimezone(timezone string) *time.Location {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// NowInTimezone returns the current time in the specified timezone.
func NowInTimezone(timezone string) time.Time {
	return time.Now().In(GetAppTimezone(timezone))
}

// FormatDuration formats a duration as HH:MM:SS.mmm.
func FormatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	milliseconds := int(d.Milliseconds()) % 1000

	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, milliseconds)
}

// FormatTimestamp formats t as YYYY-MM-DD-HH in the specified timezone.
func FormatTimestamp(t time.Time, timezone string) string {
	return t.In(GetAppTimezone(timezone)).Format(UniversalFormat)
}

// ParseTimestamp parses a YYYY-MM-DD-HH timestamp into a time.Time.
func ParseTimestamp(timestamp string, timezone string) (time.Time, error) {
	t, err := time.Parse(UniversalFormat, timestamp)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, GetAppTimezone(timezone)), nil
}

// ToAPIString formats t as YYYY-MM-DD HH:MM for API responses.
func ToAPIString(t time.Time, timezone string) string {
	return t.In(GetAppTimezone(timezone)).Format("2006-01-02 15:04")
}

// ToAPIStringOrEmpty returns formatted time string or empty string for zero time
// This prevents displaying "0001-01-01 00:17" when no recordings exist
func ToAPIStringOrEmpty(t time.Time, timezone string) string {
	if t.IsZero() {
		return ""
	}
	return t.In(GetAppTimezone(timezone)).Format("2006-01-02 15:04")
}

// ParseTimestampAsTimezone parses a timestamp and treats it as the configured timezone.
// Supports common formats: "2025-07-12T14:30:00", "2025-07-12T14:30:00Z", "2025-07-12T14:30:00+02:00", "2025-07-12 14:30:00"
// Always interprets the time as the specified timezone (ignores any timezone suffixes).
func ParseTimestampAsTimezone(timestampStr, timezone string) (time.Time, error) {
	// Try common timestamp formats, but always interpret as configured timezone
	var parsedTime time.Time
	var err error

	// Try each format in order until one succeeds
	formats := []string{
		"2006-01-02T15:04:05",       // ISO format
		"2006-01-02T15:04:05Z",      // ISO with Z
		"2006-01-02T15:04:05-07:00", // ISO with timezone offset
		"2006-01-02 15:04:05",       // Simple datetime
	}

	for _, format := range formats {
		if parsedTime, err = time.Parse(format, timestampStr); err == nil {
			break
		}
	}

	if err != nil {
		return time.Time{}, fmt.Errorf("unable to parse timestamp '%s': supported formats are ISO (2006-01-02T15:04:05) or simple (2006-01-02 15:04:05)", timestampStr)
	}

	// Always interpret the parsed time as the configured timezone
	// This ignores any timezone information in the input string
	loc := GetAppTimezone(timezone)
	localTime := time.Date(
		parsedTime.Year(), parsedTime.Month(), parsedTime.Day(),
		parsedTime.Hour(), parsedTime.Minute(), parsedTime.Second(),
		parsedTime.Nanosecond(), loc,
	)

	return localTime, nil
}

// GetCurrentHour returns the current hour in UniversalFormat (YYYY-MM-DD-HH).
// Truncates to hour boundary (sets minutes/seconds to 0) for consistent recording naming.
func GetCurrentHour(timezone string) string {
	now := NowInTimezone(timezone)
	return FormatTimestamp(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, GetAppTimezone(timezone)), timezone)
}
