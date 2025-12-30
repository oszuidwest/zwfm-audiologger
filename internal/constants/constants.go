// Package constants defines application-wide constants for recording durations,
// default configuration values, and file permissions.
package constants

import "time"

const (
	// HourlyRecordingDurationSeconds is the duration in seconds for hourly recordings.
	HourlyRecordingDurationSeconds = 3600
	// HourlyRecordingTimeout is the maximum allowed time for hourly recordings.
	HourlyRecordingTimeout = 65 * time.Minute
	// MinimumRecordingSeconds is the minimum duration in seconds for a mid-hour recording to be worthwhile.
	MinimumRecordingSeconds = 300 // 5 minutes
	// TestRecordingDurationSeconds is the duration in seconds for test recordings.
	TestRecordingDurationSeconds = 10
	// TestRecordingTimeout is the maximum allowed time for test recordings.
	TestRecordingTimeout = 30 * time.Second

	// DefaultRecordingsDir is the default directory for storing recordings.
	DefaultRecordingsDir = "/var/audio"
	// DefaultAccessLogPath is the default path for HTTP access logs.
	DefaultAccessLogPath = "/var/log/access.log"
	// DefaultKeepDays is the default number of days to retain recordings.
	DefaultKeepDays = 31
	// DefaultPort is the default HTTP server port.
	DefaultPort = 8080
	// DefaultTimezone is the default timezone for the application.
	DefaultTimezone = "UTC"

	// DirPermissions defines the file mode for created directories.
	DirPermissions = 0o755
	// FilePermissions defines the file mode for created files.
	FilePermissions = 0o644
	// LogFilePermissions defines the file mode for log files.
	LogFilePermissions = 0o640

	// DefaultDiskThresholdPercent is the default disk usage percentage threshold for alerts.
	DefaultDiskThresholdPercent = 10
	// DefaultIncompleteThresholdSeconds is the default threshold for incomplete recording alerts.
	DefaultIncompleteThresholdSeconds = 3000 // 50 minutes
	// DefaultSMTPPort is the default SMTP port for email alerts.
	DefaultSMTPPort = 587
)
