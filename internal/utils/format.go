// Package utils provides audio format detection and content type utilities
// using FFmpeg's ffprobe tool for accurate format identification.
package utils

import (
	"encoding/json"
	"os/exec"
	"strings"
)

// ProbeResult holds the ffprobe output structure
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
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-select_streams", "a:0",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		// Default to mp3 if detection fails
		return ".mp3"
	}

	var result ProbeResult
	if err := json.Unmarshal(output, &result); err != nil {
		return ".mp3"
	}

	if len(result.Streams) > 0 {
		codec := strings.ToLower(result.Streams[0].CodecName)
		switch codec {
		case "mp3", "mp3adu", "mp3adufloat", "mp3float", "mp3on4", "mp3on4float":
			return ".mp3"
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

// contentTypeMap holds MIME types for audio extensions for efficient lookup.
var contentTypeMap = map[string]string{
	".mp3":  "audio/mpeg",
	".aac":  "audio/aac",
	".m4a":  "audio/aac",
	".ogg":  "audio/ogg",
	".opus": "audio/opus",
	".flac": "audio/flac",
	".wav":  "audio/wav",
	".rec":  "application/octet-stream",
}

// ContentType returns the appropriate MIME type for an audio file extension.
// It supports common audio formats including MP3, AAC, OGG, OPUS, FLAC, and WAV.
// Uses map lookup for better performance than switch statement.
func ContentType(extension string) string {
	ext := strings.ToLower(extension)
	if mimeType, ok := contentTypeMap[ext]; ok {
		return mimeType
	}
	return "application/octet-stream"
}
