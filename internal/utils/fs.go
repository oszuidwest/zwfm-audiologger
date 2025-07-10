package utils

import (
	"os"
	"path/filepath"
)

// EnsureDir creates a directory if it doesn't exist
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// FileExists checks if a file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// StreamDir returns the directory path for a stream
func StreamDir(baseDir, streamName string) string {
	return filepath.Join(baseDir, streamName)
}

// RecordingPath returns the path for a recording file
func RecordingPath(baseDir, streamName, timestamp string) string {
	return filepath.Join(StreamDir(baseDir, streamName), timestamp+".mp3")
}

// MetadataPath returns the path for a metadata file
func MetadataPath(baseDir, streamName, timestamp string) string {
	return filepath.Join(StreamDir(baseDir, streamName), timestamp+".meta")
}