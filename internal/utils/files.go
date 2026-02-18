// Package utils provides file system utilities, FFmpeg command construction,
// audio format detection, and time utilities for audio recording management.
package utils

import (
	"fmt"
	"os"
	"path/filepath"

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
