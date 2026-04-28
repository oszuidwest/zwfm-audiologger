// Package utils provides file system utilities, FFmpeg command construction,
// audio format detection, and time utilities for audio recording management.
package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

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

// SidecarPath constructs a sidecar file path by replacing the extension.
func SidecarPath(recordingPath, newExt string) string {
	dir := filepath.Dir(recordingPath)
	baseName := strings.TrimSuffix(filepath.Base(recordingPath), filepath.Ext(recordingPath))
	return filepath.Join(dir, baseName+newExt)
}

// IsAudioFile checks if a filename has a supported audio extension.
func IsAudioFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp3", ".aac", ".ogg", ".opus", ".flac", ".m4a", ".wav":
		return true
	default:
		return false
	}
}

// AvailableDiskBytes returns the number of free bytes available on the filesystem
// containing the given path.
func AvailableDiskBytes(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	//nolint:gosec // Bsize is always positive; conversion from signed to unsigned is safe here.
	return stat.Bavail * uint64(stat.Bsize), nil
}
