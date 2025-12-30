// Package recorder handles audio stream recording functionality.
package recorder

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
)

// ensureDir creates a directory and all parent directories if they don't exist.
func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, constants.DirPermissions); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	return nil
}

// recordingPath constructs a path for a recording file.
func recordingPath(recordingsDir, stationName, timestamp, extension string) string {
	return filepath.Join(recordingsDir, stationName, timestamp+extension)
}

// cleanupTempFile removes a temporary file, logging any errors.
func cleanupTempFile(path, context string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		slog.Warn("failed to remove temp file", "file", path, "context", context, "error", err)
	}
}
