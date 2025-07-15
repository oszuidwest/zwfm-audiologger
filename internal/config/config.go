package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	RecordingsDirectory string             `json:"recordings_directory"`
	LogFile             string             `json:"log_file"`
	KeepDays            int                `json:"keep_days"`
	Debug               bool               `json:"debug"`
	Timezone            string             `json:"timezone"`
	Stations            map[string]Station `json:"stations"`
	Server              ServerConfig       `json:"server"`
}

type Station struct {
	URL              string   `json:"stream_url"`
	MetadataURL      string   `json:"metadata_url,omitempty"`
	MetadataJSONPath string   `json:"metadata_path,omitempty"`
	ParseMetadata    bool     `json:"parse_metadata,omitempty"`
	KeepDays         int      `json:"keep_days,omitempty"`
	RecordDuration   Duration `json:"record_duration,omitempty"`
}

// Duration wraps time.Duration to provide flexible JSON unmarshaling
// Accepts both string formats ("1h", "30m") and numeric nanosecond values
type Duration time.Duration

// UnmarshalJSON implements custom JSON parsing for duration values
// Supports: "1h", "30m", "45s" (string format) or raw nanoseconds (numeric)
// This allows configuration flexibility: "record_duration": "1h" or "record_duration": 3600000000000
func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	switch value := v.(type) {
	case float64:
		// Handle raw nanosecond values from JSON numbers
		*d = Duration(time.Duration(value))
	case string:
		// Parse human-readable duration strings ("1h", "30m", etc.)
		duration, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		*d = Duration(duration)
	default:
		return fmt.Errorf("invalid duration format")
	}

	return nil
}

type ServerConfig struct {
	Port            int      `json:"port"`
	ReadTimeout     Duration `json:"read_timeout"`
	WriteTimeout    Duration `json:"write_timeout"`
	ShutdownTimeout Duration `json:"shutdown_timeout"`
	CacheDirectory  string   `json:"cache_directory"`
	CacheTTL        Duration `json:"cache_ttl"`
}

// Load reads and parses a JSON configuration file.
func Load(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config.validate()
}

// validate applies defaults and validates the configuration.
func (c *Config) validate() (*Config, error) {
	if c.RecordingsDirectory == "" {
		c.RecordingsDirectory = filepath.Join(os.TempDir(), "audiologger")
	}
	if c.LogFile == "" {
		c.LogFile = "/var/log/audiologger.log"
	}
	if c.KeepDays == 0 {
		c.KeepDays = 7
	}
	if c.Timezone == "" {
		c.Timezone = "Europe/Amsterdam"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = Duration(30 * time.Second)
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = Duration(30 * time.Second)
	}
	if c.Server.ShutdownTimeout == 0 {
		c.Server.ShutdownTimeout = Duration(10 * time.Second)
	}
	if c.Server.CacheDirectory == "" {
		c.Server.CacheDirectory = filepath.Join(c.RecordingsDirectory, "cache")
	}
	if c.Server.CacheTTL == 0 {
		c.Server.CacheTTL = Duration(24 * time.Hour)
	}

	for name, station := range c.Stations {
		if station.URL == "" {
			return nil, fmt.Errorf("stream_url is required for station %s", name)
		}
		if station.KeepDays == 0 {
			station.KeepDays = c.KeepDays
		}
		if station.RecordDuration == 0 {
			station.RecordDuration = Duration(time.Hour)
		}
		c.Stations[name] = station
	}

	return c, nil
}

// GetStationKeepDays returns the retention period in days for stationName.
func (c *Config) GetStationKeepDays(stationName string) int {
	if station, exists := c.Stations[stationName]; exists && station.KeepDays > 0 {
		return station.KeepDays
	}
	return c.KeepDays
}

// GetTimezone returns the time.Location for the configured timezone.
func (c *Config) GetTimezone() (*time.Location, error) {
	return time.LoadLocation(c.Timezone)
}
