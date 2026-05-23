package utils

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// RecordCommand creates an FFmpeg command for recording audio streams with
// built-in reconnection support and timeout handling.
// ffmpegPath must be a resolvable executable; the caller is responsible for validation.
func RecordCommand(ctx context.Context, ffmpegPath, streamURL string, duration time.Duration, outputFile string) *exec.Cmd {
	args := []string{
		"-reconnect", "1", // Enable reconnection
		"-reconnect_streamed", "1", // Reconnect even for streamed protocols
		"-reconnect_delay_max", "10", // Max 10 seconds between reconnect attempts
		"-timeout", "10000000", // 10 second connection timeout (in microseconds)
		"-i", streamURL,
		"-t", strconv.FormatFloat(duration.Seconds(), 'f', -1, 64),
		"-c", "copy",
		"-y", outputFile,
	}

	//nolint:gosec // ffmpegPath comes from operator-controlled config; arguments use trusted config/internal values.
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)

	return cmd
}

// RemuxCommand creates an FFmpeg command for remuxing a file to the proper container format
// based on the output file extension, using stream copy for fast, lossless operation.
// ffmpegPath must be a resolvable executable; the caller is responsible for validation.
func RemuxCommand(ffmpegPath, inputFile, outputFile string) *exec.Cmd {
	//nolint:gosec // ffmpegPath comes from operator-controlled config; args are internal file paths.
	return exec.Command(ffmpegPath,
		"-i", inputFile,
		"-c", "copy",
		"-y", outputFile,
	)
}

// ProbeCommand creates an ffprobe command to get file metadata as JSON.
// ffprobePath must be a resolvable executable; the caller is responsible for validation.
func ProbeCommand(ctx context.Context, ffprobePath, file string) *exec.Cmd {
	//nolint:gosec // ffprobePath comes from operator-controlled config; args are internal file paths.
	return exec.CommandContext(ctx, ffprobePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		file,
	)
}

// SilenceDetectCommand creates an FFmpeg command for silence detection.
// ffmpegPath must be a resolvable executable; the caller is responsible for validation.
func SilenceDetectCommand(ctx context.Context, ffmpegPath, file string, thresholdDB int, minDurationSecs float64) *exec.Cmd {
	//nolint:gosec // ffmpegPath comes from operator-controlled config; args are internal file paths.
	return exec.CommandContext(ctx, ffmpegPath,
		"-i", file,
		"-af", fmt.Sprintf("silencedetect=noise=%ddB:d=%.1f", thresholdDB, minDurationSecs),
		"-f", "null",
		"-",
	)
}

// AudioStatsCommand creates an FFmpeg command for audio statistics extraction.
// ffmpegPath must be a resolvable executable; the caller is responsible for validation.
func AudioStatsCommand(ctx context.Context, ffmpegPath, file string) *exec.Cmd {
	//nolint:gosec // ffmpegPath comes from operator-controlled config; args are internal file paths.
	return exec.CommandContext(ctx, ffmpegPath,
		"-i", file,
		"-af", "astats=metadata=1:reset=1,ametadata=print:file=-",
		"-f", "null",
		"-",
	)
}
