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
	Validation    *ValidationConfig  `json:"validation,omitempty"`
}

// ValidationConfig holds settings for recording validation.
type ValidationConfig struct {
	Enabled            bool                `json:"enabled"`
	MinDurationSecs    int                 `json:"min_duration_secs"`
	SilenceThresholdDB float64             `json:"silence_threshold_db"`
	MaxSilenceSecs     float64             `json:"max_silence_secs"`
	MaxLoopPercent     float64             `json:"max_loop_percent"`
	Alert              *AlertConfig        `json:"alert,omitempty"`
	StationRecipients  map[string][]string `json:"station_recipients,omitempty"`
}

// AlertConfig holds settings for email alerts via Microsoft Graph.
type AlertConfig struct {
	Enabled           bool     `json:"enabled"`
	TenantID          string   `json:"tenant_id"`
	ClientID          string   `json:"client_id"`
	ClientSecret      string   `json:"client_secret"`
	SenderEmail       string   `json:"sender_email"`
	DefaultRecipients []string `json:"default_recipients,omitempty"`
}

// Station represents a radio station configuration.
type Station struct {
	StreamURL     string `json:"stream_url"`
	MetadataURL   string `json:"metadata_url,omitempty"`   // Optional metadata API endpoint
	MetadataPath  string `json:"metadata_path,omitempty"`  // JSON path for metadata extraction
	ParseMetadata bool   `json:"parse_metadata,omitempty"` // Enable JSON parsing of metadata
}

// Load reads and parses the configuration from a JSON file and applies sensible defaults for missing values.
func Load(path string) (*Config, error) {
	// Open file for streaming JSON decoding
	file, err := os.Open(path) //nolint:gosec // Config path is provided by the application, not user input
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

	// Apply defaults for validation config if enabled.
	if config.Validation != nil && config.Validation.Enabled {
		if config.Validation.MinDurationSecs == 0 {
			config.Validation.MinDurationSecs = constants.DefaultMinDurationSecs
		}
		if config.Validation.SilenceThresholdDB == 0 {
			config.Validation.SilenceThresholdDB = constants.DefaultSilenceThresholdDB
		}
		if config.Validation.MaxSilenceSecs == 0 {
			config.Validation.MaxSilenceSecs = constants.DefaultMaxSilenceSecs
		}
		if config.Validation.MaxLoopPercent == 0 {
			config.Validation.MaxLoopPercent = constants.DefaultMaxLoopPercent
		}
	}

	return &config, nil
}
