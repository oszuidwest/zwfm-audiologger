package config

import (
	"os"
	"path/filepath"
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
