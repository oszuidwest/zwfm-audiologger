package utils

import (
	"os"
	"path/filepath"
	"strings"
)

// EnsureDir creates a directory and all parent directories if they don't exist
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// BuildRecordingPath constructs a path for a recording file
func BuildRecordingPath(recordingsDir, stationName, timestamp, extension string) string {
	return filepath.Join(recordingsDir, stationName, timestamp+extension)
}

// BuildStationDir constructs the directory path for a station
func BuildStationDir(recordingsDir, stationName string) string {
	return filepath.Join(recordingsDir, stationName)
}

// FindRecordingFile looks for a recording file with any supported audio extension
// Returns the full path if found, or empty string if not found
func FindRecordingFile(recordingsDir, stationName, timestamp string) string {
	// Check for temporary .rec file first (in case rename failed)
	recPath := BuildRecordingPath(recordingsDir, stationName, timestamp, ".rec")
	if info, err := os.Stat(recPath); err == nil && !info.IsDir() {
		return recPath
	}

	// Check for properly named files
	extensions := []string{".mp3", ".aac", ".m4a", ".ogg", ".opus", ".flac", ".wav"}

	for _, ext := range extensions {
		path := BuildRecordingPath(recordingsDir, stationName, timestamp, ext)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}

	return ""
}

// GetFileExtension returns the extension of a file path
func GetFileExtension(path string) string {
	return strings.ToLower(filepath.Ext(path))
}
