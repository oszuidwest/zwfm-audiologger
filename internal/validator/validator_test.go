package validator_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
	"github.com/oszuidwest/zwfm-audiologger/internal/validator"
)

func TestMarkSkipped(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "2026-04-28-12.mp3")
	if err := os.WriteFile(audioPath, []byte("fake audio"), 0o600); err != nil {
		t.Fatal(err)
	}

	m, err := validator.New(&config.Config{RecordingsDir: dir})
	if err != nil {
		t.Fatalf("validator.New: %v", err)
	}

	const station = "teststation"
	const timestamp = "2026-04-28-12"
	m.MarkSkipped(audioPath, station, timestamp)

	sidecarPath := utils.SidecarPath(audioPath, constants.ValidationFileSuffix)

	// Sidecar must exist so that scanUnvalidated does not re-queue on restart.
	data, err := os.ReadFile(sidecarPath) //nolint:gosec // path is constructed from t.TempDir(), not user input
	if err != nil {
		t.Fatalf("sidecar not written: %v", err)
	}

	var result struct {
		Station   string `json:"station"`
		Timestamp string `json:"timestamp"`
		Valid     bool   `json:"valid"`
		Skipped   bool   `json:"skipped"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to parse sidecar JSON: %v", err)
	}
	if !result.Valid {
		t.Error("sidecar valid should be true")
	}
	if !result.Skipped {
		t.Error("sidecar skipped should be true, to distinguish from a fully validated recording")
	}
	if result.Station != station {
		t.Errorf("sidecar station = %q, want %q", result.Station, station)
	}
	if result.Timestamp != timestamp {
		t.Errorf("sidecar timestamp = %q, want %q", result.Timestamp, timestamp)
	}
}

func TestStartCancelsInFlightValidationWithoutSidecar(t *testing.T) {
	dir := t.TempDir()
	ffprobePath := filepath.Join(dir, "ffprobe")
	startedPath := filepath.Join(dir, "started")
	script := "#!/bin/sh\n: > \"" + startedPath + "\"\nexec sleep 30\n"
	if err := os.WriteFile(ffprobePath, []byte(script), 0o700); err != nil { //nolint:gosec // Test executable is written under t.TempDir().
		t.Fatalf("write ffprobe script: %v", err)
	}

	cfg := &config.Config{
		RecordingsDir: dir,
		FFmpegPath:    ffprobePath,
		FFprobePath:   ffprobePath,
		Stations:      map[string]config.Station{},
		Validation: &config.ValidationConfig{
			MinDurationSecs:    3500,
			SilenceThresholdDB: -40,
			MaxSilenceSecs:     5,
			MaxLoopPercent:     30,
		},
	}
	m, err := validator.New(cfg)
	if err != nil {
		t.Fatalf("validator.New: %v", err)
	}
	audioPath := filepath.Join(dir, "2026-04-28-12.mp3")
	m.Enqueue(audioPath, "teststation", "2026-04-28-12")

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- m.Start(ctx)
	}()

	waitForFile(t, startedPath)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not cancel the in-flight validation")
	}

	sidecarPath := utils.SidecarPath(audioPath, constants.ValidationFileSuffix)
	if _, err := os.Stat(sidecarPath); !os.IsNotExist(err) {
		t.Fatalf("validation sidecar exists after interrupted analysis: %v", err)
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("file %s was not created", path)
}
