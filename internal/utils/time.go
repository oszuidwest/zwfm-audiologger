package utils

import (
	"log/slog"
	"time"
)

const (
	// HourlyTimestampFormat is the standard format for hourly timestamps.
	HourlyTimestampFormat = "2006-01-02-15"
	// TestTimestampFormat is the format used for test recordings.
	TestTimestampFormat = "2006-01-02-15-04-05"
)

// AppTimezone holds the application timezone location. SetTimezone must be
// called during application startup, before any goroutines use this value.
var AppTimezone = time.UTC

// SetTimezone sets the application timezone from a timezone string.
// Falls back to UTC and logs an error if the timezone string is invalid.
func SetTimezone(timezoneStr string) {
	loc, err := time.LoadLocation(timezoneStr)
	if err != nil {
		slog.Error("invalid timezone in config, falling back to UTC; recording filenames will use UTC timestamps",
			"timezone", timezoneStr, "error", err)
		loc = time.UTC
	} else {
		slog.Info("Timezone set", "timezone", timezoneStr)
	}

	AppTimezone = loc
}

// Now returns the current time in the configured timezone.
func Now() time.Time {
	return time.Now().In(AppTimezone)
}

// HourlyTimestamp returns the current time formatted as an hourly timestamp in the configured timezone.
func HourlyTimestamp() string {
	return Now().Format(HourlyTimestampFormat)
}

// TestTimestamp returns the current time formatted as a test timestamp in the configured timezone.
func TestTimestamp() string {
	return Now().Format(TestTimestampFormat)
}
