// Package ffmpeg provides FFmpeg command construction and audio probing utilities.
package ffmpeg

import (
	"context"
	"encoding/json"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
)

// MaxOutputLogLength is the maximum number of bytes to include in log output.
const MaxOutputLogLength = 500

// TruncateOutput truncates output to maxLen bytes with ellipsis.
func TruncateOutput(output []byte, maxLen int) string {
	str := string(output)
	if len(str) > maxLen {
		return str[:maxLen] + "... (truncated)"
	}
	return str
}

// probeResult holds the JSON output structure for audio stream detection.
type probeResult struct {
	Streams []struct {
		CodecName string `json:"codec_name"`
	} `json:"streams"`
}

// DetectAudioFormat detects the audio format of a recorded file and returns
// the appropriate file extension based on the codec, defaulting to ".mp3"
// if detection fails or the context is cancelled.
func DetectAudioFormat(ctx context.Context, filePath string) string {
	// Run ffprobe on the file with context support
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-select_streams", "a:0",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		slog.Warn("ffprobe failed, defaulting to .mp3", "file", filePath, "error", err)
		return ".mp3"
	}

	var result probeResult
	if err := json.Unmarshal(output, &result); err != nil {
		slog.Warn("failed to parse ffprobe output, defaulting to .mp3", "file", filePath, "error", err)
		return ".mp3"
	}

	if len(result.Streams) > 0 {
		codec := strings.ToLower(result.Streams[0].CodecName)

		// Use prefix matching for codec variants (more efficient than listing all variants)
		if strings.HasPrefix(codec, "mp3") {
			return ".mp3"
		}

		switch codec {
		case "aac", "aac_latm":
			return ".aac"
		case "vorbis":
			return ".ogg"
		case "opus":
			return ".opus"
		case "flac":
			return ".flac"
		default:
			// Use codec name as extension if unknown
			return "." + codec
		}
	}

	return ".mp3" // Default fallback
}

// probeFormatResult holds the JSON output structure for media format information.
type probeFormatResult struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// ProbeDuration returns the duration of a media file in seconds.
// It returns the duration as a float64, or an error if the duration cannot be determined.
// The context parameter allows the caller to cancel or timeout the operation.
func ProbeDuration(ctx context.Context, filePath string) (float64, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	var result probeFormatResult
	if err := json.Unmarshal(output, &result); err != nil {
		return 0, err
	}

	if result.Format.Duration == "" {
		return 0, nil
	}

	duration, err := strconv.ParseFloat(result.Format.Duration, 64)
	if err != nil {
		return 0, err
	}

	return duration, nil
}
