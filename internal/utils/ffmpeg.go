// Package utils provides FFmpeg command construction utilities.
package utils

import (
	"context"
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

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	return cmd
}

// TrimCommand creates an FFmpeg command for extracting a specific time range
// from an audio file using stream copy for fast, lossless operation.
func TrimCommand(inputFile, startOffset, duration, outputFile string) *exec.Cmd {
	return exec.Command("ffmpeg",
		"-i", inputFile,
		"-ss", startOffset,
		"-t", duration,
		"-c", "copy",
		"-y", outputFile,
	)
}

// RemuxCommand creates an FFmpeg command for remuxing a file to the proper container format
// based on the output file extension, using stream copy for fast, lossless operation.
func RemuxCommand(inputFile, outputFile string) *exec.Cmd {
	return exec.Command("ffmpeg",
		"-i", inputFile,
		"-c", "copy",
		"-y", outputFile,
	)
}
