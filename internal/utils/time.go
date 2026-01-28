package utils

import (
	"log/slog"
	"sync"
	"time"
)

const (
	// HourlyTimestampFormat is the standard format for hourly timestamps.
	HourlyTimestampFormat = "2006-01-02-15"
	// TestTimestampFormat is the format used for test recordings.
	TestTimestampFormat = "2006-01-02-15-04-05"
)

var (
	// AppTimezone holds the application timezone location.
	AppTimezone *time.Location = time.UTC
	// timezoneMutex protects AppTimezone reads/writes.
	timezoneMutex sync.RWMutex
)

// SetTimezone sets the application timezone from a timezone string.
func SetTimezone(timezoneStr string) error {
	loc, err := time.LoadLocation(timezoneStr)
	if err != nil {
		slog.Warn("failed to load timezone, using UTC", "timezone", timezoneStr, "error", err)
		timezoneMutex.Lock()
		AppTimezone = time.UTC
		timezoneMutex.Unlock()
		return err
	}
	timezoneMutex.Lock()
	AppTimezone = loc
	timezoneMutex.Unlock()
	slog.Info("Timezone set", "timezone", timezoneStr)
	return nil
}

// Now returns the current time in the configured timezone.
func Now() time.Time {
	timezoneMutex.RLock()
	tz := AppTimezone
	timezoneMutex.RUnlock()
	return time.Now().In(tz)
}

// HourlyTimestamp returns the current time formatted as an hourly timestamp in the configured timezone.
func HourlyTimestamp() string {
	return Now().Format(HourlyTimestampFormat)
}

// TestTimestamp returns the current time formatted as a test timestamp in the configured timezone.
func TestTimestamp() string {
	return Now().Format(TestTimestampFormat)
}

// ParseHourlyTimestamp parses a timestamp in hourly format and returns it in the configured timezone.
func ParseHourlyTimestamp(timestamp string) (time.Time, error) {
	t, err := time.Parse(HourlyTimestampFormat, timestamp)
	if err != nil {
		return time.Time{}, err
	}
	// Convert to the configured timezone
	return t.In(AppTimezone), nil
}
