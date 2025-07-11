// Package utils provides file system utilities for audio recording management,
// including directory creation, file path construction, and file discovery.
package utils

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
)

// EnsureDir creates a directory and all parent directories if they don't exist.
func EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, constants.DirPermissions); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	return nil
}

// RecordingPath constructs a path for a recording file.
func RecordingPath(recordingsDir, stationName, timestamp, extension string) string {
	return filepath.Join(recordingsDir, stationName, timestamp+extension)
}

// FindRecordingFile locates a recording file by timestamp and station.
func FindRecordingFile(recordingsDir, stationName, timestamp string) (string, error) {
	// Check extensions in priority order: mkv first (temp files), then common formats
	priorityExtensions := []string{".mkv", ".mp3", ".aac", ".m4a", ".ogg", ".opus", ".flac", ".wav"}

	for _, ext := range priorityExtensions {
		path := RecordingPath(recordingsDir, stationName, timestamp, ext)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, nil
		}
	}

	return "", fs.ErrNotExist
}

// Extension returns the extension of a file path.
func Extension(path string) string {
	return strings.ToLower(filepath.Ext(path))
}
