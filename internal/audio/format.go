// Package audio provides audio format definitions and codec detection utilities
// for supporting multiple audio formats in the audio logger.
package audio

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	ffmpeglib "github.com/u2takey/ffmpeg-go"
)

// Format represents an audio format with its properties including extension,
// MIME type, and codec information.
type Format struct {
	Name      string
	Extension string
	MimeType  string
	Codec     string
}

// AudioInfo contains complete audio file information from ffprobe analysis.
type AudioInfo struct {
	Duration       time.Duration
	DurationString string
	Bitrate        int
	BitrateKbps    int
	Codec          string
	Format         Format
}

// FormatMP3 defines the MP3 audio format configuration.
var FormatMP3 = Format{
	Name:      "mp3",
	Extension: ".mp3",
	MimeType:  "audio/mpeg",
	Codec:     "mp3",
}

// FormatAAC defines the AAC audio format configuration.
var FormatAAC = Format{
	Name:      "aac",
	Extension: ".aac",
	MimeType:  "audio/aac",
	Codec:     "aac",
}

// FormatM4A defines the M4A audio format configuration.
var FormatM4A = Format{
	Name:      "m4a",
	Extension: ".m4a",
	MimeType:  "audio/mp4",
	Codec:     "aac",
}

// DetectFormatFromCodec returns the appropriate format based on codec name.
func DetectFormatFromCodec(codec string) (Format, error) {
	codec = strings.ToLower(codec)
	switch codec {
	case "mp3":
		return FormatMP3, nil
	case "aac":
		return FormatAAC, nil
	default:
		return Format{}, fmt.Errorf("unsupported codec: %s", codec)
	}
}

// formatDuration formats a duration as HH:MM:SS.mmm
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	milliseconds := int(d.Milliseconds()) % 1000

	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, milliseconds)
}

// ProbeFile analyzes an audio file using ffprobe and returns comprehensive audio information.
func ProbeFile(filePath string) (*AudioInfo, error) {
	return ProbeWithOptions(filePath, nil)
}

// ProbeWithOptions analyzes an audio file or stream using ffprobe with custom options.
func ProbeWithOptions(input string, options ffmpeglib.KwArgs) (*AudioInfo, error) {
	// Use ffprobe to analyze the audio file or stream
	var probeData string
	var err error

	if options != nil {
		probeData, err = ffmpeglib.Probe(input, options)
	} else {
		probeData, err = ffmpeglib.Probe(input)
	}

	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	// Parse JSON output
	var probeOutput struct {
		Streams []struct {
			Duration  string `json:"duration"`
			BitRate   string `json:"bit_rate"`
			CodecName string `json:"codec_name"`
		} `json:"streams"`
	}

	if err := json.Unmarshal([]byte(probeData), &probeOutput); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	if len(probeOutput.Streams) == 0 {
		return nil, fmt.Errorf("no streams found in recording")
	}

	audioStream := probeOutput.Streams[0]

	// Parse duration (may be empty for live streams)
	var duration time.Duration
	if audioStream.Duration != "" {
		durationSeconds, err := strconv.ParseFloat(audioStream.Duration, 64)
		if err != nil {
			return nil, fmt.Errorf("unable to parse duration: %w", err)
		}
		duration = time.Duration(durationSeconds * float64(time.Second))
	}

	// Parse bitrate
	bitrate := 0
	if audioStream.BitRate != "" {
		if br, err := strconv.Atoi(audioStream.BitRate); err == nil {
			bitrate = br
		}
	}

	// Detect format from codec
	format, err := DetectFormatFromCodec(audioStream.CodecName)
	if err != nil {
		return nil, fmt.Errorf("unsupported codec %s: %w", audioStream.CodecName, err)
	}

	return &AudioInfo{
		Duration:       duration,
		DurationString: formatDuration(duration),
		Bitrate:        bitrate,
		BitrateKbps:    bitrate / 1000,
		Codec:          audioStream.CodecName,
		Format:         format,
	}, nil
}
