package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Config represents the entire application configuration
type Config struct {
	RecordingDir string            `json:"recording_dir"`
	LogFile      string            `json:"log_file"`
	KeepDays     int               `json:"keep_days"`
	Debug        bool              `json:"debug"`
	Streams      map[string]Stream `json:"streams"`
	Server       ServerConfig      `json:"server"`
}

// Stream represents a single stream configuration
type Stream struct {
	URL              string   `json:"stream_url"`
	MetadataURL      string   `json:"metadata_url,omitempty"`
	MetadataJSONPath string   `json:"metadata_path,omitempty"`
	ParseMetadata    bool     `json:"parse_metadata,omitempty"`
	KeepDays         int      `json:"keep_days,omitempty"`
	RecordDuration   Duration `json:"record_duration,omitempty"`
}

// Duration wraps time.Duration to provide custom JSON unmarshaling
type Duration time.Duration

// UnmarshalJSON implements json.Unmarshaler for Duration
func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	switch value := v.(type) {
	case float64:
		*d = Duration(time.Duration(value))
	case string:
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

// ServerConfig represents HTTP server configuration
type ServerConfig struct {
	Port            int      `json:"port"`
	ReadTimeout     Duration `json:"read_timeout"`
	WriteTimeout    Duration `json:"write_timeout"`
	ShutdownTimeout Duration `json:"shutdown_timeout"`
	CacheDir        string   `json:"cache_dir"`
	CacheTTL        Duration `json:"cache_ttl"`
}

// Load loads configuration from a JSON file
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

// validate validates and sets defaults for the configuration
func (c *Config) validate() (*Config, error) {
	// Set defaults
	if c.RecordingDir == "" {
		c.RecordingDir = filepath.Join(os.TempDir(), "audiologger")
	}
	if c.LogFile == "" {
		c.LogFile = filepath.Join(c.RecordingDir, "audiologger.log")
	}
	if c.KeepDays == 0 {
		c.KeepDays = 7
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
	if c.Server.CacheDir == "" {
		c.Server.CacheDir = filepath.Join(c.RecordingDir, "cache")
	}
	if c.Server.CacheTTL == 0 {
		c.Server.CacheTTL = Duration(24 * time.Hour)
	}

	// Validate streams
	for name, stream := range c.Streams {
		if stream.URL == "" {
			return nil, fmt.Errorf("stream_url is required for stream %s", name)
		}
		if stream.KeepDays == 0 {
			stream.KeepDays = c.KeepDays
		}
		if stream.RecordDuration == 0 {
			stream.RecordDuration = Duration(time.Hour)
		}
		c.Streams[name] = stream
	}

	return c, nil
}

// GetStreamKeepDays returns the keep_days setting for a stream
func (c *Config) GetStreamKeepDays(streamName string) int {
	if stream, exists := c.Streams[streamName]; exists && stream.KeepDays > 0 {
		return stream.KeepDays
	}
	return c.KeepDays
}
