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

// ContentType returns the appropriate MIME type for an audio file extension.
// It supports common audio formats including MP3, AAC, OGG, OPUS, FLAC, and WAV.
func ContentType(extension string) string {
	switch strings.ToLower(extension) {
	case ".mp3":
		return "audio/mpeg"
	case ".aac", ".m4a":
		return "audio/aac"
	case ".ogg":
		return "audio/ogg"
	case ".opus":
		return "audio/opus"
	case ".flac":
		return "audio/flac"
	case ".wav":
		return "audio/wav"
	case ".rec":
		return "application/octet-stream"
	default:
		return "application/octet-stream"
	}
}
