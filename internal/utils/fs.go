package utils

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnsureDirectory creates dir and all necessary parent directories.
func EnsureDirectory(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// FileExists reports whether path exists.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// StationDirectory returns the full path to a station's directory.
func StationDirectory(baseDir, stationName string) string {
	return filepath.Join(baseDir, stationName)
}

// RecordingPath returns the full path to a recording file.
func RecordingPath(baseDir, stationName, timestamp string) string {
	return filepath.Join(StationDirectory(baseDir, stationName), timestamp+".mp3")
}

// MetadataPath returns the full path to a metadata file.
func MetadataPath(baseDir, stationName, timestamp string) string {
	return filepath.Join(StationDirectory(baseDir, stationName), timestamp+".meta")
}

// WrapError wraps an error with additional context
func WrapError(err error, message string) error {
	return fmt.Errorf("%s: %w", message, err)
}

// WrapErrorf wraps an error with formatted context
func WrapErrorf(err error, format string, args ...interface{}) error {
	return fmt.Errorf(format+": %w", append(args, err)...)
}
