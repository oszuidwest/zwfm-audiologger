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

	cmd := exec.CommandContext(ctx, ffmpegPath, args...) //nolint:gosec // Binary path is validated at startup; arguments are constructed from trusted config values.

	return cmd
}

// RemuxCommand creates an FFmpeg command for remuxing a file to the proper container format
// based on the output file extension, using stream copy for fast, lossless operation.
func RemuxCommand(ffmpegPath, inputFile, outputFile string) *exec.Cmd {
	return exec.Command(ffmpegPath, //nolint:gosec // Binary path is validated at startup; args are from internal file paths, not user HTTP input.
		"-i", inputFile,
		"-c", "copy",
		"-y", outputFile,
	)
}

// ProbeCommand creates an ffprobe command to get file metadata as JSON.
func ProbeCommand(ctx context.Context, ffprobePath, file string) *exec.Cmd {
	return exec.CommandContext(ctx, ffprobePath, //nolint:gosec // Binary path is validated at startup; args are from internal file paths.
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		file,
	)
}

// SilenceDetectCommand creates an FFmpeg command for silence detection.
func SilenceDetectCommand(ctx context.Context, ffmpegPath, file string, thresholdDB int, minDurationSecs float64) *exec.Cmd {
	return exec.CommandContext(ctx, ffmpegPath, //nolint:gosec // Binary path is validated at startup; args are from internal file paths.
		"-i", file,
		"-af", fmt.Sprintf("silencedetect=noise=%ddB:d=%.1f", thresholdDB, minDurationSecs),
		"-f", "null",
		"-",
	)
}

// AudioStatsCommand creates an FFmpeg command for audio statistics extraction.
func AudioStatsCommand(ctx context.Context, ffmpegPath, file string) *exec.Cmd {
	return exec.CommandContext(ctx, ffmpegPath, //nolint:gosec // Binary path is validated at startup; args are from internal file paths.
		"-i", file,
		"-af", "astats=metadata=1:reset=1,ametadata=print:file=-",
		"-f", "null",
		"-",
	)
}
