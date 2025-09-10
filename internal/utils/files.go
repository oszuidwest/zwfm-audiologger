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

// supportedExtensions lists all audio file extensions supported by the recording system.
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

// FindRecordingFile looks for a recording file using Go 1.25's enhanced fs.Glob
func FindRecordingFile(recordingsDir, stationName, timestamp string) (string, error) {
	// Check for temporary .rec file first (in case rename failed)
	recPath := RecordingPath(recordingsDir, stationName, timestamp, ".rec")
	if info, err := os.Stat(recPath); err == nil && info.Mode().IsRegular() {
		return recPath, nil
	}

	// Use Go 1.25's fs.Glob for efficient file pattern matching
	stationDir := StationDir(recordingsDir, stationName)
	fsys := os.DirFS(stationDir)

	// Create glob pattern for the timestamp with any supported extension
	pattern := timestamp + ".*"

	matches, err := fs.Glob(fsys, pattern)
	if err != nil {
		return "", fmt.Errorf("failed to glob files: %w", err)
	}

	if len(matches) == 0 {
		return "", fs.ErrNotExist
	}

	// Filter matches to only supported extensions and return first match
	for _, match := range matches {
		ext := filepath.Ext(match)
		if slices.Contains(supportedExtensions, ext) {
			return filepath.Join(stationDir, match), nil
		}
	}

	return "", fs.ErrNotExist
}

// Extension returns the extension of a file path
func Extension(path string) string {
	return strings.ToLower(filepath.Ext(path))
}
