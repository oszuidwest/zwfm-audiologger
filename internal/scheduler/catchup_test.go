package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
)

func TestCatchupRemaining(t *testing.T) {
	tests := []struct {
		name       string
		minute     int
		sec        int
		wantSecs   int
		wantNeeded bool
	}{
		// Start of hour: full catchup, well above the minimum threshold.
		{"start of hour", 0, 0, 3600, true},
		// One second in: still needed.
		{"one second in", 0, 1, 3599, true},
		// Exactly at the minimum boundary (60 s remaining): still needed.
		{"at minimum boundary", 59, 0, 60, true},
		// One second past the minimum (59 s remaining): skip.
		{"one second past minimum", 59, 1, 59, false},
		// Near end of hour: definitely skip.
		{"near end of hour", 59, 59, 1, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 4, 28, 12, tc.minute, tc.sec, 0, time.UTC)
			gotSecs, gotNeeded := catchupRemaining(now)
			if gotSecs != tc.wantSecs {
				t.Errorf("remainingSecs = %d, want %d", gotSecs, tc.wantSecs)
			}
			if gotNeeded != tc.wantNeeded {
				t.Errorf("needed = %v, want %v", gotNeeded, tc.wantNeeded)
			}
		})
	}
}

func TestExistingAudioFile(t *testing.T) {
	const ts = "2026-04-28-12"

	t.Run("non-existent dir", func(t *testing.T) {
		f, err := existingAudioFile(filepath.Join(t.TempDir(), "nodir"), ts)
		if err != nil {
			t.Fatalf("unexpected error for non-existent dir: %v", err)
		}
		if f != "" {
			t.Errorf("got %q, want empty string", f)
		}
	})

	t.Run("empty dir", func(t *testing.T) {
		f, err := existingAudioFile(t.TempDir(), ts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f != "" {
			t.Errorf("got %q, want empty string", f)
		}
	})

	// Side-car files must not be mistaken for audio files.
	for _, sidecar := range []string{ts + ".meta", ts + constants.ValidationFileSuffix} {
		sidecar := sidecar
		t.Run("sidecar only: "+sidecar, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, sidecar), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			f, err := existingAudioFile(dir, ts)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if f != "" {
				t.Errorf("sidecar %q must not count as audio; got %q", sidecar, f)
			}
		})
	}

	// Every supported audio extension must be recognised.
	for _, ext := range []string{".mp3", ".aac", ".ogg", ".opus", ".flac", ".m4a", ".wav"} {
		ext := ext
		t.Run("audio file: "+ext, func(t *testing.T) {
			dir := t.TempDir()
			want := ts + ext
			if err := os.WriteFile(filepath.Join(dir, want), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			f, err := existingAudioFile(dir, ts)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if f != want {
				t.Errorf("got %q, want %q", f, want)
			}
		})
	}

	t.Run("different timestamp audio ignored", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "2026-04-28-11.mp3"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := existingAudioFile(dir, ts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f != "" {
			t.Errorf("different-timestamp file should not match; got %q", f)
		}
	})
}
