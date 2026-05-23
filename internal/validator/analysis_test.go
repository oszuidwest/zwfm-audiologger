package validator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
)

func TestAnalyzeDurationUsesConfiguredFFprobePath(t *testing.T) {
	ffprobePath := filepath.Join(t.TempDir(), "ffprobe")
	writeAnalysisScript(t, ffprobePath, `#!/bin/sh
printf '%s\n' '{"format":{"duration":"12.5"}}'
`)

	m := &Manager{config: &config.Config{FFprobePath: ffprobePath}}

	got, err := m.analyzeDuration(context.Background(), "recording.mp3")
	if err != nil {
		t.Fatalf("analyzeDuration returned error: %v", err)
	}
	if got != 12.5 {
		t.Fatalf("analyzeDuration = %v, want 12.5", got)
	}
}

func TestAnalyzeSilenceUsesConfiguredFFmpegPath(t *testing.T) {
	ffmpegPath := filepath.Join(t.TempDir(), "ffmpeg")
	writeAnalysisScript(t, ffmpegPath, `#!/bin/sh
printf '%s\n' 'silence_start: 1 silence_end: 7 silence_duration: 6' >&2
exit 1
`)

	m := &Manager{config: &config.Config{
		FFmpegPath: ffmpegPath,
		Validation: &config.ValidationConfig{
			SilenceThresholdDB: -40,
			MaxSilenceSecs:     5,
		},
	}}

	got, err := m.analyzeSilence(context.Background(), "recording.mp3")
	if err != nil {
		t.Fatalf("analyzeSilence returned error: %v", err)
	}
	if got != 6 {
		t.Fatalf("analyzeSilence = %v, want 6", got)
	}
}

func TestAnalyzeLoopsUsesConfiguredFFmpegPath(t *testing.T) {
	ffmpegPath := filepath.Join(t.TempDir(), "ffmpeg")
	writeAnalysisScript(t, ffmpegPath, `#!/bin/sh
i=0
while [ "$i" -lt 60 ]; do
	printf '%s\n' 'RMS_level=-1'
	i=$((i + 1))
done
exit 1
`)

	m := &Manager{config: &config.Config{FFmpegPath: ffmpegPath}}

	got, err := m.analyzeLoops(context.Background(), "recording.mp3")
	if err != nil {
		t.Fatalf("analyzeLoops returned error: %v", err)
	}
	if got != 100 {
		t.Fatalf("analyzeLoops = %v, want 100", got)
	}
}

func writeAnalysisScript(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o700); err != nil { //nolint:gosec // Test executable is written under t.TempDir().
		t.Fatalf("write analysis script %s: %v", path, err)
	}
}
