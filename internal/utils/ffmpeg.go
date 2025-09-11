// Package utils provides FFmpeg command construction utilities for audio
// recording with reconnection support and audio trimming operations.
package utils

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
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

	var cmd *exec.Cmd
	if ctx != nil {
		cmd = exec.CommandContext(ctx, "ffmpeg", args...)
	} else {
		cmd = exec.Command("ffmpeg", args...)
	}

	// DEBUG: Log the exact command being executed
	fmt.Printf("EXEC DEBUG: Full command: %s %s\n", cmd.Path, strings.Join(args, " "))
	fmt.Printf("EXEC DEBUG: Context: %v\n", ctx)
	fmt.Printf("EXEC DEBUG: Working dir: %s\n", func() string {
		if wd, err := os.Getwd(); err == nil {
			return wd
		}
		return "unknown"
	}())

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
