// Package timeutil provides timezone-aware time utilities for timestamp formatting.
package timeutil

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

// appTimezone holds the application timezone location.
// This is set once at startup before any goroutines access it.
var appTimezone = time.UTC

// SetTimezone sets the application timezone from a timezone string.
// Must be called before starting any goroutines that use LocalNow().
func SetTimezone(timezoneStr string) error {
	loc, err := time.LoadLocation(timezoneStr)
	if err != nil {
		slog.Warn("failed to load timezone, using UTC", "timezone", timezoneStr, "error", err)
		appTimezone = time.UTC
		return err
	}
	appTimezone = loc
	slog.Info("Timezone set", "timezone", timezoneStr)
	return nil
}

// Timezone returns the configured timezone location.
func Timezone() *time.Location {
	return appTimezone
}

// LocalNow returns the current time in the configured timezone.
func LocalNow() time.Time {
	return time.Now().In(appTimezone)
}

// HourlyTimestamp returns the current time formatted as an hourly timestamp in the configured timezone.
func HourlyTimestamp() string {
	return LocalNow().Format(HourlyTimestampFormat)
}

// TestTimestamp returns the current time formatted as a test timestamp in the configured timezone.
func TestTimestamp() string {
	return LocalNow().Format(TestTimestampFormat)
}
