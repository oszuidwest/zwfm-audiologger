// Package config handles application configuration loading and validation
package config

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
	"github.com/spf13/viper"
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

// Load reads and parses the configuration from a JSON file using Viper.
// It supports JSON configuration files and provides sensible defaults
// for missing configuration values.
func Load(path string) (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("recordings_dir", "/var/audio")
	v.SetDefault("keep_days", 31)
	v.SetDefault("port", 8080)
	v.SetDefault("timezone", "UTC")

	// Configure viper
	dir := filepath.Dir(path)
	fileName := filepath.Base(path)
	ext := filepath.Ext(fileName)
	name := strings.TrimSuffix(fileName, ext)

	v.SetConfigName(name)
	v.SetConfigType(strings.TrimPrefix(ext, "."))
	v.AddConfigPath(dir)
	v.AddConfigPath(".")

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		return nil, utils.LogError(context.Background(), "read config file", err)
	}

	// Unmarshal into struct
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, utils.LogError(context.Background(), "parse config", err)
	}

	return &config, nil
}
