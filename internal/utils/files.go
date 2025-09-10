// Package utils provides file system utilities for audio recording management,
// including directory creation, file path construction, and file discovery.
package utils

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var supportedExtensions = []string{".mp3", ".aac", ".m4a", ".ogg", ".opus", ".flac", ".wav"}

// EnsureDir creates a directory and all parent directories if they don't exist
func EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	return nil
}

// RecordingPath constructs a path for a recording file
func RecordingPath(recordingsDir, stationName, timestamp, extension string) string {
	return filepath.Join(recordingsDir, stationName, timestamp+extension)
}

// StationDir constructs the directory path for a station
func StationDir(recordingsDir, stationName string) string {
	return filepath.Join(recordingsDir, stationName)
}

// FindRecordingFile looks for a recording file with modern error handling
func FindRecordingFile(recordingsDir, stationName, timestamp string) (string, error) {
	// Check for temporary .rec file first (in case rename failed)
	recPath := RecordingPath(recordingsDir, stationName, timestamp, ".rec")
	if info, err := os.Stat(recPath); err == nil && !info.IsDir() {
		return recPath, nil
	}

	// Use Go 1.25's improved error handling
	var foundFiles []string

	for _, ext := range supportedExtensions {
		path := RecordingPath(recordingsDir, stationName, timestamp, ext)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			foundFiles = append(foundFiles, path)
		}
	}

	switch len(foundFiles) {
	case 0:
		return "", fs.ErrNotExist
	case 1:
		return foundFiles[0], nil
	default:
		// Multiple files found - return the first supported one
		slices.Sort(foundFiles) // Deterministic ordering
		return foundFiles[0], nil
	}
}

// Extension returns the extension of a file path
func Extension(path string) string {
	return strings.ToLower(filepath.Ext(path))
}
