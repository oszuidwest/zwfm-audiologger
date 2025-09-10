package utils

import "time"

const (
	// HourlyTimestampFormat is the standard format for hourly timestamps
	HourlyTimestampFormat = "2006-01-02-15"
	// TestTimestampFormat is the format used for test recordings
	TestTimestampFormat = "2006-01-02-15-04-05"
)

// HourlyTimestamp returns the current time formatted as an hourly timestamp
func HourlyTimestamp() string {
	return time.Now().Format(HourlyTimestampFormat)
}

// TestTimestamp returns the current time formatted as a test timestamp
func TestTimestamp() string {
	return time.Now().Format(TestTimestampFormat)
}

// ParseHourlyTimestamp parses a timestamp in hourly format
func ParseHourlyTimestamp(timestamp string) (time.Time, error) {
	return time.Parse(HourlyTimestampFormat, timestamp)
}
