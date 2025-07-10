package utils

import (
	"fmt"
	"time"
)

// Universal time format: YYYY-MM-DD-HH used everywhere
// This ensures universal interpretation across the entire application
const (
	UniversalFormat = "2006-01-02-15" // YYYY-MM-DD-HH (files, API, everything)
	DisplayFormat   = "02-01-2006 15:04" // DD-MM-YYYY HH:MM (dashboard only)
)

// GetAppTimezone returns the application timezone (always Europe/Amsterdam)
func GetAppTimezone() *time.Location {
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		// Fallback to UTC if timezone loading fails
		return time.UTC
	}
	return loc
}

// Now returns current time in application timezone
func Now() time.Time {
	return time.Now().In(GetAppTimezone())
}

// FormatDuration formats a duration for FFmpeg (HH:MM:SS.mmm format)
func FormatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	milliseconds := int(d.Milliseconds()) % 1000

	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, milliseconds)
}

// FormatTimestamp formats a time using universal format (YYYY-MM-DD-HH)
func FormatTimestamp(t time.Time) string {
	return t.In(GetAppTimezone()).Format(UniversalFormat)
}

// ParseTimestamp parses a universal timestamp (YYYY-MM-DD-HH)
func ParseTimestamp(timestamp string) (time.Time, error) {
	t, err := time.Parse(UniversalFormat, timestamp)
	if err != nil {
		return time.Time{}, err
	}
	// Ensure parsed time is in application timezone
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, GetAppTimezone()), nil
}

// ToAPIString converts time to API response format (YYYY-MM-DD HH:MM)
func ToAPIString(t time.Time) string {
	return t.In(GetAppTimezone()).Format("2006-01-02 15:04")
}

// GetCurrentHour returns the current hour timestamp (YYYY-MM-DD-HH)
func GetCurrentHour() string {
	now := Now()
	return FormatTimestamp(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, GetAppTimezone()))
}

