package utils

import (
	"fmt"
	"os"
	"path/filepath"
)

func EnsureDirectory(dir string) error {
	return os.MkdirAll(dir, 0755)
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func StationDirectory(baseDir, stationName string) string {
	return filepath.Join(baseDir, stationName)
}

func RecordingPath(baseDir, stationName, timestamp string) string {
	return filepath.Join(StationDirectory(baseDir, stationName), timestamp+".mp3")
}

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
