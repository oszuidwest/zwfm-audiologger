package utils

import (
	"log"
	"time"
)

const (
	// HourlyTimestampFormat is the standard format for hourly timestamps
	HourlyTimestampFormat = "2006-01-02-15"
	// TestTimestampFormat is the format used for test recordings
	TestTimestampFormat = "2006-01-02-15-04-05"
)

var (
	// AppTimezone holds the application timezone location
	AppTimezone *time.Location = time.UTC
)

// SetTimezone sets the application timezone from a timezone string
func SetTimezone(timezoneStr string) error {
	loc, err := time.LoadLocation(timezoneStr)
	if err != nil {
		log.Printf("Failed to load timezone %s: %v, using UTC", timezoneStr, err)
		AppTimezone = time.UTC
		return err
	}
	AppTimezone = loc
	log.Printf("Timezone set to: %s", timezoneStr)
	return nil
}

// Now returns the current time in the configured timezone
func Now() time.Time {
	return time.Now().In(AppTimezone)
}

// HourlyTimestamp returns the current time formatted as an hourly timestamp in the configured timezone
func HourlyTimestamp() string {
	return Now().Format(HourlyTimestampFormat)
}

// TestTimestamp returns the current time formatted as a test timestamp in the configured timezone
func TestTimestamp() string {
	return Now().Format(TestTimestampFormat)
}

// ParseHourlyTimestamp parses a timestamp in hourly format and returns it in the configured timezone
func ParseHourlyTimestamp(timestamp string) (time.Time, error) {
	t, err := time.Parse(HourlyTimestampFormat, timestamp)
	if err != nil {
		return time.Time{}, err
	}
	// Convert to the configured timezone
	return t.In(AppTimezone), nil
}
