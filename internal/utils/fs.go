// Package utils provides file system utilities and audio file handling functions
// for the audio logger application.
package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oszuidwest/zwfm-audiologger/internal/audio"
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

// RecordingPath returns the full path to a recording file with the given format.
func RecordingPath(baseDir, stationName, timestamp string, format audio.Format) string {
	return filepath.Join(StationDirectory(baseDir, stationName), timestamp+format.Extension)
}

// FindRecordingFile finds an existing recording file with any supported format.
func FindRecordingFile(baseDir, stationName, timestamp string) (string, audio.Format, bool) {
	stationDir := StationDirectory(baseDir, stationName)

	// Try each supported format directly
	formats := []audio.Format{audio.FormatMP3, audio.FormatAAC, audio.FormatM4A}
	for _, format := range formats {
		path := filepath.Join(stationDir, timestamp+format.Extension)
		if FileExists(path) {
			return path, format, true
		}
	}

	return "", audio.Format{}, false
}

// MetadataPath returns the full path to a metadata file.
func MetadataPath(baseDir, stationName, timestamp string) string {
	return filepath.Join(StationDirectory(baseDir, stationName), timestamp+".meta")
}

// IsAudioFile checks if a file has a supported audio format extension.
func IsAudioFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".mp3" || ext == ".aac" || ext == ".m4a"
}

// GetTimestampFromAudioFile extracts timestamp from audio filename.
func GetTimestampFromAudioFile(filename string) (string, audio.Format, error) {
	if strings.HasSuffix(filename, ".mp3") {
		return strings.TrimSuffix(filename, ".mp3"), audio.FormatMP3, nil
	}
	if strings.HasSuffix(filename, ".aac") {
		return strings.TrimSuffix(filename, ".aac"), audio.FormatAAC, nil
	}
	if strings.HasSuffix(filename, ".m4a") {
		return strings.TrimSuffix(filename, ".m4a"), audio.FormatM4A, nil
	}
	return "", audio.Format{}, fmt.Errorf("unrecognized audio file format: %s", filename)
}
