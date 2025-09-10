// Package utils provides FFmpeg command construction utilities for audio
// recording with reconnection support and audio trimming operations.
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

	if ctx != nil {
		return exec.CommandContext(ctx, "ffmpeg", args...)
	}
	return exec.Command("ffmpeg", args...)
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
