// Package config handles application configuration loading.
package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
)

// Config represents the application configuration.
type Config struct {
	RecordingsDir string             `json:"recordings_dir"`
	Port          int                `json:"port"`
	KeepDays      int                `json:"keep_days"`
	Timezone      string             `json:"timezone"`
	Stations      map[string]Station `json:"stations"`
}

// Station represents a radio station configuration.
type Station struct {
	StreamURL     string `json:"stream_url"`
	APISecret     string `json:"api_secret,omitempty"` // Per-station API secret
	MetadataURL   string `json:"metadata_url,omitempty"`
	MetadataPath  string `json:"metadata_path,omitempty"`
	ParseMetadata bool   `json:"parse_metadata,omitempty"`
}

// Load reads and parses the configuration from a JSON file and applies sensible defaults for missing values.
func Load(path string) (*Config, error) {
	// Open file for streaming JSON decoding
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	var config Config

	// Parse JSON configuration from file
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields() // Strict validation - fail on unexpected fields

	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults for missing values
	if config.RecordingsDir == "" {
		config.RecordingsDir = constants.DefaultRecordingsDir
	}
	if config.KeepDays == 0 {
		config.KeepDays = constants.DefaultKeepDays
	}
	if config.Port == 0 {
		config.Port = constants.DefaultPort
	}
	if config.Timezone == "" {
		config.Timezone = constants.DefaultTimezone
	}

	return &config, nil
}
