package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// probeResult holds the ffprobe output structure.
type probeResult struct {
	Streams []struct {
		CodecName string `json:"codec_name"`
	} `json:"streams"`
}

// Format uses ffprobe to detect the actual format of a recorded audio file.
// It returns the appropriate file extension based on the detected codec.
// ffprobePath must be a resolvable executable; the caller is responsible for validation.
func Format(ctx context.Context, ffprobePath, filePath string) (string, error) {
	// Run ffprobe on the file
	//nolint:gosec // ffprobePath comes from operator-controlled config; args are internal file paths.
	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-select_streams", "a:0",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("run ffprobe: %w", err)
	}

	var result probeResult
	if err := json.Unmarshal(output, &result); err != nil {
		return "", fmt.Errorf("parse ffprobe output: %w", err)
	}

	if len(result.Streams) > 0 {
		return extensionForCodec(result.Streams[0].CodecName), nil
	}

	return "", errors.New("ffprobe returned no audio streams")
}

func extensionForCodec(codecName string) string {
	codec := strings.ToLower(codecName)
	if codec == "" {
		return ".mp3"
	}

	// Use prefix matching for codec variants (more efficient than listing all variants).
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
		if !isSafeCodecName(codec) {
			return ".mp3"
		}
		return "." + codec
	}
}

func isSafeCodecName(codec string) bool {
	for _, r := range codec {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

// contentTypeMap maps file extensions to their MIME types.
var contentTypeMap = map[string]string{
	".mp3":  "audio/mpeg",
	".aac":  "audio/aac",
	".m4a":  "audio/aac",
	".ogg":  "audio/ogg",
	".opus": "audio/opus",
	".flac": "audio/flac",
	".wav":  "audio/wav",
}

// ContentType returns the appropriate MIME type for an audio file extension.
// It supports common audio formats including MP3, AAC, OGG, OPUS, FLAC, and WAV.
func ContentType(extension string) string {
	ext := strings.ToLower(extension)
	if mimeType, ok := contentTypeMap[ext]; ok {
		return mimeType
	}
	return "application/octet-stream"
}
