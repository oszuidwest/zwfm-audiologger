// Package config handles application configuration loading.
package config

import (
	"cmp"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
)

// Config represents the application configuration.
type Config struct {
	RecordingsDir string             `json:"recordings_dir"`
	Port          int                `json:"port"`
	KeepDays      int                `json:"keep_days"`
	Timezone      string             `json:"timezone"`
	FFmpegPath    string             `json:"ffmpeg_path"`
	FFprobePath   string             `json:"ffprobe_path"`
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
	data, err := os.ReadFile(path) //nolint:gosec // Config path is provided by the application, not user input.
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("parse config file %q: %w", path, err)
	}

	cfg.applyDefaults()
	return &cfg, nil
}

// Validate verifies that FFmpegPath and FFprobePath resolve via exec.LookPath.
// It does not validate other configuration fields.
func (c *Config) Validate() error {
	if c.FFmpegPath == "" {
		return errors.New("ffmpeg_path must not be empty")
	}
	if _, err := exec.LookPath(c.FFmpegPath); err != nil {
		return fmt.Errorf("ffmpeg binary not found at %q: ensure ffmpeg_path in config.json points to a valid binary: %w", c.FFmpegPath, err)
	}

	if c.FFprobePath == "" {
		return errors.New("ffprobe_path must not be empty")
	}
	if _, err := exec.LookPath(c.FFprobePath); err != nil {
		return fmt.Errorf("ffprobe binary not found at %q: ensure ffprobe_path in config.json points to a valid binary: %w", c.FFprobePath, err)
	}

	return nil
}

func (c *Config) applyDefaults() {
	c.RecordingsDir = cmp.Or(c.RecordingsDir, constants.DefaultRecordingsDir)
	c.KeepDays = cmp.Or(c.KeepDays, constants.DefaultKeepDays)
	c.Port = cmp.Or(c.Port, constants.DefaultPort)
	c.Timezone = cmp.Or(c.Timezone, constants.DefaultTimezone)
	c.FFmpegPath = cmp.Or(c.FFmpegPath, constants.DefaultFFmpegPath)
	c.FFprobePath = cmp.Or(c.FFprobePath, constants.DefaultFFprobePath)

	if c.Validation != nil && c.Validation.Enabled {
		c.Validation.applyDefaults()
	}
}

func (v *ValidationConfig) applyDefaults() {
	v.MinDurationSecs = cmp.Or(v.MinDurationSecs, constants.DefaultMinDurationSecs)
	v.SilenceThresholdDB = cmp.Or(v.SilenceThresholdDB, constants.DefaultSilenceThresholdDB)
	v.MaxSilenceSecs = cmp.Or(v.MaxSilenceSecs, constants.DefaultMaxSilenceSecs)
	v.MaxLoopPercent = cmp.Or(v.MaxLoopPercent, constants.DefaultMaxLoopPercent)
}
