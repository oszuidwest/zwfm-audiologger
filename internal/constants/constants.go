// Package constants defines application-wide constants for recording durations,
// default configuration values, and file permissions.
package constants

import "time"

const (
	// HourlyRecordingDuration is the duration in seconds for hourly recordings.
	HourlyRecordingDuration = "3600"
	// HourlyRecordingTimeout is the maximum allowed time for hourly recordings.
	HourlyRecordingTimeout = 65 * time.Minute
	// TestRecordingDuration is the duration in seconds for test recordings.
	TestRecordingDuration = "10"
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

	// DefaultMinDurationSecs is the minimum expected recording duration in seconds.
	DefaultMinDurationSecs = 3500
	// DefaultSilenceThresholdDB is the default silence detection threshold in dB.
	DefaultSilenceThresholdDB = -40.0
	// DefaultMaxSilenceSecs is the maximum allowed continuous silence duration.
	DefaultMaxSilenceSecs = 5.0
	// DefaultMaxLoopPercent is the maximum allowed percentage of looped content.
	DefaultMaxLoopPercent = 30.0
	// ValidationQueueSize is the capacity of the validation job queue.
	ValidationQueueSize = 100
	// ValidationAnalysisTimeout is the maximum time allowed for validation analysis.
	ValidationAnalysisTimeout = 10 * time.Minute
)
