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

func TestValidateAcceptsBinariesFromPATH(t *testing.T) {
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, "ffmpeg"))
	writeExecutable(t, filepath.Join(dir, "ffprobe"))
	t.Setenv("PATH", dir)

	cfg := &Config{
		FFmpegPath:  "ffmpeg",
		FFprobePath: "ffprobe",
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateRequiresConfiguredPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *Config
		want string
	}{
		{
			name: "empty ffmpeg path",
			cfg:  &Config{FFprobePath: validExecutablePath(t)},
			want: "ffmpeg_path must not be empty",
		},
		{
			name: "empty ffprobe path",
			cfg:  &Config{FFmpegPath: validExecutablePath(t)},
			want: "ffprobe_path must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cfg.Validate()
			assertErrorContains(t, err, tt.want)
		})
	}
}

func TestValidateRequiresFFmpeg(t *testing.T) {
	t.Parallel()

	cfg := validTestConfig(t)
	cfg.FFmpegPath = filepath.Join(t.TempDir(), "missing-ffmpeg")

	err := cfg.Validate()
	assertErrorContains(t, err, "ffmpeg binary not found", cfg.FFmpegPath, "ffmpeg_path")
}

func TestValidateRequiresFFprobe(t *testing.T) {
	t.Parallel()

	cfg := validTestConfig(t)
	cfg.FFprobePath = filepath.Join(t.TempDir(), "missing-ffprobe")

	err := cfg.Validate()
	assertErrorContains(t, err, "ffprobe binary not found", cfg.FFprobePath, "ffprobe_path")
}

func validTestConfig(t *testing.T) *Config {
	t.Helper()

	executablePath := validExecutablePath(t)

	return &Config{
		FFmpegPath:  executablePath,
		FFprobePath: executablePath,
	}
}

func validExecutablePath(t *testing.T) string {
	t.Helper()

	executablePath, err := os.Executable()
	if err != nil {
		t.Fatalf("test executable path: %v", err)
	}

	return executablePath
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()

	data := []byte("#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(path, data, 0o700); err != nil { //nolint:gosec // Test executable is written under t.TempDir().
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func assertErrorContains(t *testing.T, err error, parts ...string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error")
	}

	failed := false
	for _, part := range parts {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("expected error to contain %q, got: %v", part, err)
			failed = true
		}
	}
	if failed {
		t.FailNow()
	}
}
