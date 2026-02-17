package utils

import (
	"encoding/json"
	"log/slog"
	"os/exec"
	"strings"
)

// ProbeResult holds the ffprobe output structure.
type ProbeResult struct {
	Streams []struct {
		CodecName string `json:"codec_name"`
	} `json:"streams"`
}

// Format uses ffprobe to detect the actual format of a recorded audio file.
// It returns the appropriate file extension based on the detected codec,
// defaulting to ".mp3" if detection fails.
func Format(filePath string) string {
	// Run ffprobe on the file
	cmd := exec.Command("ffprobe", //nolint:gosec // G204: args are from internal file paths, not user HTTP input
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

	var result ProbeResult
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
