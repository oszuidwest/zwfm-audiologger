// Package ffmpeg provides FFmpeg command construction and audio probing utilities.
package ffmpeg

import (
	"context"
	"os/exec"
	"syscall"
	"time"
)

// gracefulShutdownDelay is the time to wait for FFmpeg to exit gracefully after SIGTERM.
const gracefulShutdownDelay = 5 * time.Second

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

	// Graceful shutdown: send SIGTERM first, wait 5s, then SIGKILL
	// This allows FFmpeg to flush buffers and finalize the output file
	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = gracefulShutdownDelay

	return cmd
}

// RemuxCommand creates an FFmpeg command for remuxing a file to the proper container format
// based on the output file extension, using stream copy for fast, lossless operation.
func RemuxCommand(ctx context.Context, inputFile, outputFile string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", inputFile,
		"-c", "copy",
		"-y", outputFile,
	)

	// Graceful shutdown for remux as well
	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = gracefulShutdownDelay

	return cmd
}
