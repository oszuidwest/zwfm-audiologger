// Package config handles application configuration loading
package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config represents the application configuration
type Config struct {
	RecordingsDir string             `json:"recordings_dir"`
	Port          int                `json:"port"`
	KeepDays      int                `json:"keep_days"`
	Timezone      string             `json:"timezone"`
	Stations      map[string]Station `json:"stations"`
}

// Station represents a radio station configuration
type Station struct {
	StreamURL     string `json:"stream_url"`
	APISecret     string `json:"api_secret,omitempty"` // Per-station API secret
	MetadataURL   string `json:"metadata_url,omitempty"`
	MetadataPath  string `json:"metadata_path,omitempty"`
	ParseMetadata bool   `json:"parse_metadata,omitempty"`
}

// Load reads and parses the configuration from a JSON file using streaming decoder.
// It provides sensible defaults for missing configuration values.
// Uses Go 1.25's enhanced JSON streaming performance for better memory efficiency.
func Load(path string) (*Config, error) {
	// Open file for streaming JSON decoding
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var config Config

	// Use streaming JSON decoder for better performance and memory efficiency
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields() // Strict validation - fail on unexpected fields

	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults for missing values
	if config.RecordingsDir == "" {
		config.RecordingsDir = "/var/audio"
	}
	if config.KeepDays == 0 {
		config.KeepDays = 31
	}
	if config.Port == 0 {
		config.Port = 8080
	}
	if config.Timezone == "" {
		config.Timezone = "UTC"
	}

	return &config, nil
}
