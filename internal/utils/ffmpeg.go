package utils

import (
	"context"
	"os/exec"
)

// FFmpegRecordCommand creates a new FFmpeg command for recording with reconnection support
func FFmpegRecordCommand(ctx context.Context, streamURL, duration, outputFile string) *exec.Cmd {
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

// FFmpegTrimCommand creates a new FFmpeg command for trimming audio
func FFmpegTrimCommand(inputFile, startOffset, duration, outputFile string) *exec.Cmd {
	return exec.Command("ffmpeg",
		"-i", inputFile,
		"-ss", startOffset,
		"-t", duration,
		"-c", "copy",
		"-y", outputFile,
	)
}
