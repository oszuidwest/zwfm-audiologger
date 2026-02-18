package utils

import (
	"context"
	"fmt"
	"os/exec"
)

// RecordCommand creates an FFmpeg command for recording audio streams with
// built-in reconnection support and timeout handling.
func RecordCommand(ctx context.Context, streamURL, duration, outputFile string) *exec.Cmd {
	args := []string{
		"-reconnect", "1", // Enable reconnection
		"-reconnect_streamed", "1", // Reconnect even for streamed protocols
		"-reconnect_delay_max", "10", // Max 10 seconds between reconnect attempts
		"-timeout", "10000000", // 10 second connection timeout (in microseconds)
		"-i", streamURL,
		"-t", duration,
		"-c", "copy",
		"-y", outputFile,
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...) //nolint:gosec // Arguments are constructed from trusted config values

	return cmd
}

// RemuxCommand creates an FFmpeg command for remuxing a file to the proper container format
// based on the output file extension, using stream copy for fast, lossless operation.
func RemuxCommand(inputFile, outputFile string) *exec.Cmd {
	return exec.Command("ffmpeg", //nolint:gosec // G204: args are from internal file paths, not user HTTP input
		"-i", inputFile,
		"-c", "copy",
		"-y", outputFile,
	)
}

// ProbeCommand creates an ffprobe command to get file metadata as JSON.
func ProbeCommand(ctx context.Context, file string) *exec.Cmd {
	return exec.CommandContext(ctx, "ffprobe", //nolint:gosec // G204: args are from internal file paths
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		file,
	)
}

// SilenceDetectCommand creates an FFmpeg command for silence detection.
func SilenceDetectCommand(ctx context.Context, file string, thresholdDB int, minDurationSecs float64) *exec.Cmd {
	return exec.CommandContext(ctx, "ffmpeg", //nolint:gosec // G204: args are from internal file paths
		"-i", file,
		"-af", fmt.Sprintf("silencedetect=noise=%ddB:d=%.1f", thresholdDB, minDurationSecs),
		"-f", "null",
		"-",
	)
}

// AudioStatsCommand creates an FFmpeg command for audio statistics extraction.
func AudioStatsCommand(ctx context.Context, file string) *exec.Cmd {
	return exec.CommandContext(ctx, "ffmpeg", //nolint:gosec // G204: args are from internal file paths
		"-i", file,
		"-af", "astats=metadata=1:reset=1,ametadata=print:file=-",
		"-f", "null",
		"-",
	)
}
