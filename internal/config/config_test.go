package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
)

func TestLoadAppliesDefaultsAndParsesStations(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	data := []byte(`{
  "stations": {
    "station1": {
      "stream_url": "https://stream.example.com/station1.mp3",
      "metadata_url": "https://api.example.com/nowplaying",
      "metadata_path": "data.current.title",
      "parse_metadata": true
    }
  }
}`)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.RecordingsDir != constants.DefaultRecordingsDir {
		t.Errorf("RecordingsDir = %q, want %q", cfg.RecordingsDir, constants.DefaultRecordingsDir)
	}
	if cfg.Port != constants.DefaultPort {
		t.Errorf("Port = %d, want %d", cfg.Port, constants.DefaultPort)
	}
	if cfg.KeepDays != constants.DefaultKeepDays {
		t.Errorf("KeepDays = %d, want %d", cfg.KeepDays, constants.DefaultKeepDays)
	}
	if cfg.Timezone != constants.DefaultTimezone {
		t.Errorf("Timezone = %q, want %q", cfg.Timezone, constants.DefaultTimezone)
	}
	if cfg.FFmpegPath != constants.DefaultFFmpegPath {
		t.Errorf("FFmpegPath = %q, want %q", cfg.FFmpegPath, constants.DefaultFFmpegPath)
	}
	if cfg.FFprobePath != constants.DefaultFFprobePath {
		t.Errorf("FFprobePath = %q, want %q", cfg.FFprobePath, constants.DefaultFFprobePath)
	}

	station := cfg.Stations["station1"]
	if station.StreamURL != "https://stream.example.com/station1.mp3" {
		t.Errorf("StreamURL = %q", station.StreamURL)
	}
	if station.MetadataURL != "https://api.example.com/nowplaying" {
		t.Errorf("MetadataURL = %q", station.MetadataURL)
	}
	if station.MetadataPath != "data.current.title" {
		t.Errorf("MetadataPath = %q", station.MetadataPath)
	}
	if !station.ParseMetadata {
		t.Error("ParseMetadata = false, want true")
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"stations": {}, "unexpected": true}`)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(configPath); err == nil {
		t.Fatal("Load returned nil error for config with an unknown field")
	}
}

func TestValidateAcceptsConfiguredBinaries(t *testing.T) {
	t.Parallel()

	cfg := validTestConfig(t)

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateRequiresFFmpeg(t *testing.T) {
	t.Parallel()

	cfg := validTestConfig(t)
	cfg.FFmpegPath = "/definitely/missing/ffmpeg"

	err := cfg.Validate()
	assertErrorContains(t, err, "ffmpeg binary not found", "/definitely/missing/ffmpeg", "ffmpeg_path")
}

func TestValidateRequiresFFprobe(t *testing.T) {
	t.Parallel()

	cfg := validTestConfig(t)
	cfg.FFprobePath = "/definitely/missing/ffprobe"

	err := cfg.Validate()
	assertErrorContains(t, err, "ffprobe binary not found", "/definitely/missing/ffprobe", "ffprobe_path")
}

func validTestConfig(t *testing.T) *Config {
	t.Helper()

	executablePath, err := os.Executable()
	if err != nil {
		t.Fatalf("test executable path: %v", err)
	}

	return &Config{
		FFmpegPath:  executablePath,
		FFprobePath: executablePath,
	}
}

func assertErrorContains(t *testing.T, err error, parts ...string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error")
	}

	for _, part := range parts {
		if !strings.Contains(err.Error(), part) {
			t.Fatalf("expected error to contain %q, got: %v", part, err)
		}
	}
}
