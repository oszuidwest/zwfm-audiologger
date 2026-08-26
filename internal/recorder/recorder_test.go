package recorder

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
)

type recordingFailureNotifier struct {
	calls  atomic.Int32
	reason atomic.Value
}

func (n *recordingFailureNotifier) NotifyRecordingFailure(_, reason string) {
	n.calls.Add(1)
	n.reason.Store(reason)
}

func (n *recordingFailureNotifier) lastReason() string {
	reason := n.reason.Load()
	if reason == nil {
		return ""
	}
	return reason.(string)
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

func TestRecordUsesConfiguredProbeAndRemuxPaths(t *testing.T) {
	recordingsDir := t.TempDir()
	binDir := t.TempDir()
	ffprobePath := filepath.Join(binDir, "ffprobe")
	ffmpegPath := filepath.Join(binDir, "ffmpeg")
	probeMarker := filepath.Join(binDir, "probe.marker")
	remuxMarker := filepath.Join(binDir, "remux.marker")
	t.Setenv("FAKE_FFPROBE_MARKER", probeMarker)
	t.Setenv("FAKE_FFMPEG_MARKER", remuxMarker)

	writeShellScript(t, ffprobePath, `#!/bin/sh
printf probe > "$FAKE_FFPROBE_MARKER"
printf '%s\n' '{"streams":[{"codec_name":"aac"}]}'
`)
	writeShellScript(t, ffmpegPath, `#!/bin/sh
printf remux > "$FAKE_FFMPEG_MARKER"
while [ "$#" -gt 1 ]; do
	shift
done
printf remuxed > "$1"
`)

	manager := New(&config.Config{
		RecordingsDir: recordingsDir,
		FFmpegPath:    ffmpegPath,
		FFprobePath:   ffprobePath,
	}, nil, nil)
	manager.recordCommand = func(ctx context.Context, _ string, _ time.Duration, outputFile string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestRecorderHelperProcess", "--", outputFile) //nolint:gosec // Test helper process and temp output path are controlled by this test.
		cmd.Env = append(os.Environ(), "GO_WANT_RECORDER_WRITE_HELPER_PROCESS=1")
		return cmd
	}
	manager.availableBytes = func(string) (uint64, error) {
		return constants.MinDiskSpaceBytes, nil
	}

	const timestamp = "2026-04-30-23"
	manager.record(context.Background(), recordOptions{
		name:      "station",
		station:   &config.Station{StreamURL: "https://stream.example.com/station.aac"},
		timestamp: timestamp,
		duration:  time.Second,
		timeout:   5 * time.Second,
	})

	if _, err := os.Stat(probeMarker); err != nil {
		t.Fatalf("configured ffprobe was not executed: %v", err)
	}
	if _, err := os.Stat(remuxMarker); err != nil {
		t.Fatalf("configured ffmpeg was not executed for remux: %v", err)
	}
	finalFile := utils.RecordingPath(recordingsDir, "station", timestamp, ".aac")
	if _, err := os.Stat(finalFile); err != nil {
		t.Fatalf("final recording not written at detected format path: %v", err)
	}
}

func TestRecordNotifiesAndKeepsTempFileWhenFormatDetectionFails(t *testing.T) {
	recordingsDir := t.TempDir()
	binDir := t.TempDir()
	ffprobePath := filepath.Join(binDir, "ffprobe")
	ffmpegPath := filepath.Join(binDir, "ffmpeg")
	remuxMarker := filepath.Join(binDir, "remux.marker")
	t.Setenv("FAKE_FFMPEG_MARKER", remuxMarker)

	writeShellScript(t, ffprobePath, "#!/bin/sh\nexit 1\n")
	writeShellScript(t, ffmpegPath, `#!/bin/sh
printf remux > "$FAKE_FFMPEG_MARKER"
exit 0
`)

	notifier := &recordingFailureNotifier{}
	manager := New(&config.Config{
		RecordingsDir: recordingsDir,
		FFmpegPath:    ffmpegPath,
		FFprobePath:   ffprobePath,
	}, nil, notifier)
	manager.recordCommand = func(ctx context.Context, _ string, _ time.Duration, outputFile string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestRecorderHelperProcess", "--", outputFile) //nolint:gosec // Test helper process and temp output path are controlled by this test.
		cmd.Env = append(os.Environ(), "GO_WANT_RECORDER_WRITE_HELPER_PROCESS=1")
		return cmd
	}
	manager.availableBytes = func(string) (uint64, error) {
		return constants.MinDiskSpaceBytes, nil
	}

	const timestamp = "2026-04-30-23"
	manager.record(context.Background(), recordOptions{
		name:      "station",
		station:   &config.Station{StreamURL: "https://stream.example.com/station.mp3"},
		timestamp: timestamp,
		duration:  time.Second,
		timeout:   5 * time.Second,
	})

	if got := notifier.calls.Load(); got != 1 {
		t.Fatalf("NotifyRecordingFailure calls = %d, want 1", got)
	}
	if reason := notifier.lastReason(); !strings.Contains(reason, "format detection failed") {
		t.Fatalf("NotifyRecordingFailure reason = %q, want format detection failure", reason)
	}
	tempFile := utils.RecordingPath(recordingsDir, "station", timestamp, ".mkv")
	if _, err := os.Stat(tempFile); err != nil {
		t.Fatalf("temporary recording was not kept after format detection failure: %v", err)
	}
	if _, err := os.Stat(remuxMarker); !os.IsNotExist(err) {
		t.Fatalf("remux command ran after format detection failure; stat error: %v", err)
	}
}

func TestRecordKeepsTempFileAndRemovesPartialOutputWhenRemuxFails(t *testing.T) {
	recordingsDir := t.TempDir()
	binDir := t.TempDir()
	ffprobePath := filepath.Join(binDir, "ffprobe")
	ffmpegPath := filepath.Join(binDir, "ffmpeg")

	writeShellScript(t, ffprobePath, `#!/bin/sh
printf '%s\n' '{"streams":[{"codec_name":"aac"}]}'
`)
	// Write a partial output file, then fail, like an interrupted remux would.
	writeShellScript(t, ffmpegPath, `#!/bin/sh
while [ "$#" -gt 1 ]; do
	shift
done
printf partial > "$1"
exit 1
`)

	notifier := &recordingFailureNotifier{}
	manager := New(&config.Config{
		RecordingsDir: recordingsDir,
		FFmpegPath:    ffmpegPath,
		FFprobePath:   ffprobePath,
	}, nil, notifier)
	manager.recordCommand = func(ctx context.Context, _ string, _ time.Duration, outputFile string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestRecorderHelperProcess", "--", outputFile) //nolint:gosec // Test helper process and temp output path are controlled by this test.
		cmd.Env = append(os.Environ(), "GO_WANT_RECORDER_WRITE_HELPER_PROCESS=1")
		return cmd
	}
	manager.availableBytes = func(string) (uint64, error) {
		return constants.MinDiskSpaceBytes, nil
	}

	const timestamp = "2026-04-30-23"
	manager.record(context.Background(), recordOptions{
		name:      "station",
		station:   &config.Station{StreamURL: "https://stream.example.com/station.aac"},
		timestamp: timestamp,
		duration:  time.Second,
		timeout:   5 * time.Second,
	})

	if got := notifier.calls.Load(); got != 1 {
		t.Fatalf("NotifyRecordingFailure calls = %d, want 1", got)
	}
	if reason := notifier.lastReason(); !strings.Contains(reason, "remux failed") {
		t.Fatalf("NotifyRecordingFailure reason = %q, want remux failure", reason)
	}
	tempFile := utils.RecordingPath(recordingsDir, "station", timestamp, ".mkv")
	if _, err := os.Stat(tempFile); err != nil {
		t.Fatalf("temporary recording was not kept after remux failure: %v", err)
	}
	finalFile := utils.RecordingPath(recordingsDir, "station", timestamp, ".aac")
	if _, err := os.Stat(finalFile); !os.IsNotExist(err) {
		t.Fatalf("partial remux output was not removed; stat error: %v", err)
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
	shouldSleep := os.Getenv("GO_WANT_RECORDER_HELPER_PROCESS") == "1"
	shouldExit := os.Getenv("GO_WANT_RECORDER_WRITE_HELPER_PROCESS") == "1"
	if !shouldSleep && !shouldExit {
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

	if shouldExit {
		os.Exit(0)
	}
	time.Sleep(24 * time.Hour)
}

func writeShellScript(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o700); err != nil { //nolint:gosec // Test executable is written under t.TempDir().
		t.Fatalf("write shell script %s: %v", path, err)
	}
}
