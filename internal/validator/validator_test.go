package validator_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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

	m := validator.New(&config.Config{RecordingsDir: dir})
	t.Cleanup(m.Stop)

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
