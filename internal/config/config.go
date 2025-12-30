// Package config handles application configuration loading.
package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"

	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
)

// Config represents the application configuration.
type Config struct {
	RecordingsDir string             `json:"recordings_dir"`
	Port          int                `json:"port"`
	KeepDays      int                `json:"keep_days"`
	Timezone      string             `json:"timezone"`
	Alerting      AlertingConfig     `json:"alerting"`
	Stations      map[string]Station `json:"stations"`
}

// AlertingConfig represents the alerting configuration.
type AlertingConfig struct {
	WebhookURL                 string      `json:"webhook_url"`
	Email                      EmailConfig `json:"email"`
	DiskThresholdPercent       int         `json:"disk_threshold_percent"`
	IncompleteThresholdSeconds int         `json:"incomplete_threshold_seconds"`
}

// EmailConfig represents the email alerting configuration.
type EmailConfig struct {
	Enabled  bool     `json:"enabled"`
	SMTPHost string   `json:"smtp_host"`
	SMTPPort int      `json:"smtp_port"`
	Username string   `json:"smtp_user"`
	Password string   `json:"smtp_pass"`
	FromAddr string   `json:"from"`
	ToAddrs  []string `json:"to"`
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
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	// Initialize config with defaults BEFORE decoding
	// JSON decode will only override fields that are present in the file
	config := Config{
		RecordingsDir: constants.DefaultRecordingsDir,
		Port:          constants.DefaultPort,
		KeepDays:      constants.DefaultKeepDays,
		Timezone:      constants.DefaultTimezone,
		Alerting: AlertingConfig{
			DiskThresholdPercent:       constants.DefaultDiskThresholdPercent,
			IncompleteThresholdSeconds: constants.DefaultIncompleteThresholdSeconds,
			Email: EmailConfig{
				SMTPPort: constants.DefaultSMTPPort,
			},
		},
	}

	// Parse JSON configuration from file (overrides defaults)
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields() // Strict validation - fail on unexpected fields

	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate alerting configuration
	if err := validateAlertingConfig(&config.Alerting); err != nil {
		return nil, err
	}

	return &config, nil
}

// validateAlertingConfig validates the alerting configuration.
func validateAlertingConfig(alerting *AlertingConfig) error {
	// Validate webhook URL if set
	if alerting.WebhookURL != "" {
		parsedURL, err := url.Parse(alerting.WebhookURL)
		if err != nil {
			return fmt.Errorf("alerting.webhook_url is invalid: %w", err)
		}
		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			return fmt.Errorf("alerting.webhook_url must use http or https scheme, got %q", parsedURL.Scheme)
		}
		if parsedURL.Host == "" {
			return fmt.Errorf("alerting.webhook_url must have a valid host")
		}
	}

	// Validate email configuration if enabled
	if alerting.Email.Enabled {
		if alerting.Email.SMTPHost == "" {
			return fmt.Errorf("alerting.email.smtp_host is required when email alerting is enabled")
		}
		if alerting.Email.FromAddr == "" {
			return fmt.Errorf("alerting.email.from is required when email alerting is enabled")
		}
		if len(alerting.Email.ToAddrs) == 0 {
			return fmt.Errorf("alerting.email.to must contain at least one recipient when email alerting is enabled")
		}
	}

	// Validate disk threshold percent (1-99)
	if alerting.DiskThresholdPercent < 1 || alerting.DiskThresholdPercent > 99 {
		return fmt.Errorf("alerting.disk_threshold_percent must be between 1 and 99, got %d", alerting.DiskThresholdPercent)
	}

	// Validate incomplete threshold seconds (positive and less than 3600)
	if alerting.IncompleteThresholdSeconds < 1 {
		return fmt.Errorf("alerting.incomplete_threshold_seconds must be positive, got %d", alerting.IncompleteThresholdSeconds)
	}
	if alerting.IncompleteThresholdSeconds >= 3600 {
		return fmt.Errorf("alerting.incomplete_threshold_seconds must be less than 3600 (1 hour), got %d", alerting.IncompleteThresholdSeconds)
	}

	return nil
}
