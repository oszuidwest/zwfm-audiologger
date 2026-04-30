package recorder

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
)

type recordingFailureNotifier struct {
	calls atomic.Int32
}

func (n *recordingFailureNotifier) NotifyRecordingFailure(_, _ string) {
	n.calls.Add(1)
}

func TestScheduledAndCatchupDoNotNotifyOnParentContextCancellation(t *testing.T) {
	tests := []struct {
		name      string
		timestamp string
		run       func(context.Context, *Manager, *config.Station, string)
	}{
		{
			name:      "scheduled",
			timestamp: utils.HourlyTimestamp(),
			run: func(ctx context.Context, m *Manager, station *config.Station, _ string) {
				m.Scheduled(ctx, "station", station)
			},
		},
		{
			name:      "catchup",
			timestamp: "2026-04-30-23",
			run: func(ctx context.Context, m *Manager, station *config.Station, timestamp string) {
				m.Catchup(ctx, "station", station, timestamp, 3600)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recordingsDir := t.TempDir()
			notifier := &recordingFailureNotifier{}
			manager := New(&config.Config{RecordingsDir: recordingsDir}, nil, notifier)
			manager.recordCommand = func(ctx context.Context, _ string, _ time.Duration, outputFile string) *exec.Cmd {
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestRecorderHelperProcess", "--", outputFile) //nolint:gosec // Test helper process and temp output path are controlled by this test.
				cmd.Env = append(os.Environ(), "GO_WANT_RECORDER_HELPER_PROCESS=1")
				return cmd
			}
			manager.availableBytes = func(string) (uint64, error) {
				return constants.MinDiskSpaceBytes, nil
			}
			station := &config.Station{StreamURL: "https://stream.example.com/station.mp3"}
			tempFile := utils.RecordingPath(recordingsDir, "station", tt.timestamp, ".mkv")

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				defer close(done)
				tt.run(ctx, manager, station, tt.timestamp)
			}()
			defer func() {
				cancel()
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					t.Fatal("recording did not stop during test cleanup")
				}
			}()

			waitForFile(t, tempFile)
			cancel()

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("recording did not stop after parent context cancellation")
			}

			if got := notifier.calls.Load(); got != 0 {
				t.Fatalf("NotifyRecordingFailure calls = %d, want 0", got)
			}
			if _, err := os.Stat(tempFile); !os.IsNotExist(err) {
				t.Fatalf("temporary file was not removed after cancellation; stat error: %v", err)
			}
		})
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
	t.Fatalf("timed out waiting for %s", path)
}

func TestRecorderHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_RECORDER_HELPER_PROCESS") != "1" {
		return
	}

	if len(os.Args) < 3 {
		os.Exit(2)
	}
	outputFile := os.Args[len(os.Args)-1]
	if err := os.MkdirAll(filepath.Dir(outputFile), 0o750); err != nil { //nolint:gosec // Path is the test-controlled temp output passed to the helper process.
		os.Exit(2)
	}
	if err := os.WriteFile(outputFile, []byte("partial recording"), 0o600); err != nil { //nolint:gosec // Path is the test-controlled temp output passed to the helper process.
		os.Exit(2)
	}

	time.Sleep(24 * time.Hour)
}
