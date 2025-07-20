// Package peaks provides functionality for generating waveform peak data
// from audio files for visualization purposes (like WaveSurfer.js).
package peaks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/oszuidwest/zwfm-audiologger/internal/logger"
)

const (
	// DefaultSamplesPerPixel is the default zoom level for web visualization
	// 800 samples per pixel gives about 4500 data points for a 1-hour recording
	DefaultSamplesPerPixel = 800
)

// PeaksData represents the waveform peaks for an audio file
type PeaksData struct {
	Version         int       `json:"version"`
	Channels        int       `json:"channels"`
	SampleRate      int       `json:"sample_rate"`
	SamplesPerPixel int       `json:"samples_per_pixel"`
	Bits            int       `json:"bits"`
	Length          int       `json:"length"`
	Data            []float64 `json:"data"`
}

// Generator handles the generation of waveform peaks data
type Generator struct {
	logger *logger.Logger
}

// NewGenerator creates a new peaks generator
func NewGenerator(logger *logger.Logger) *Generator {
	return &Generator{
		logger: logger,
	}
}

// ExtractPeaksData extracts raw peaks data from an audio file
// samplesPerPixel determines the zoom level (lower = more detail)
func (g *Generator) ExtractPeaksData(audioPath string, samplesPerPixel int) (*PeaksData, error) {
	// Validate input file exists
	if _, err := os.Stat(audioPath); err != nil {
		return nil, fmt.Errorf("audio file not found: %w", err)
	}

	// Get audio info first
	info, err := g.getAudioInfo(audioPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get audio info: %w", err)
	}

	// Generate peaks using FFmpeg
	peaks, err := g.extractPeaks(audioPath, samplesPerPixel, info.sampleRate, info.channels)
	if err != nil {
		return nil, fmt.Errorf("failed to extract peaks: %w", err)
	}

	return &PeaksData{
		Version:         2,
		Channels:        info.channels,
		SampleRate:      info.sampleRate,
		SamplesPerPixel: samplesPerPixel,
		Bits:            8, // We normalize to 8-bit for web visualization
		Length:          len(peaks),
		Data:            peaks,
	}, nil
}

// GetPeaksFilePath returns the path where peaks data should be stored for a recording
func GetPeaksFilePath(recordingPath string) string {
	// Add .peaks.json extension to the recording file
	return recordingPath + ".peaks.json"
}

// Generate creates and saves peaks data for an audio file
// This is used when you want to generate peaks without checking if they exist
func (g *Generator) Generate(audioPath string, samplesPerPixel int) (*PeaksData, error) {
	// Generate peaks data
	peaksData, err := g.ExtractPeaksData(audioPath, samplesPerPixel)
	if err != nil {
		return nil, fmt.Errorf("failed to generate peaks: %w", err)
	}

	// Save to file
	peaksPath := GetPeaksFilePath(audioPath)
	if err := g.SaveToFile(peaksData, peaksPath); err != nil {
		// Log error but return the data anyway since generation succeeded
		g.logger.Error("failed to save peaks file", "path", peaksPath, "error", err)
	}

	return peaksData, nil
}

// GetPeaks retrieves peaks data, generating it if necessary
// Returns the peaks data and whether it was newly generated
func (g *Generator) GetPeaks(audioPath string, samplesPerPixel int) (*PeaksData, bool, error) {
	peaksPath := GetPeaksFilePath(audioPath)

	// Try to load existing peaks file
	if peaksData, err := g.LoadFromFile(peaksPath); err == nil {
		return peaksData, false, nil // false = not newly generated
	}

	// Generate new peaks if file doesn't exist or can't be loaded
	g.logger.Info("generating peaks on-demand", "path", audioPath)
	peaksData, err := g.Generate(audioPath, samplesPerPixel)
	if err != nil {
		return nil, false, err
	}

	return peaksData, true, nil // true = newly generated
}

// SaveToFile saves peaks data to a JSON file
func (g *Generator) SaveToFile(peaks *PeaksData, outputPath string) error {
	// Ensure directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Marshal to JSON
	data, err := json.Marshal(peaks)
	if err != nil {
		return fmt.Errorf("failed to marshal peaks data: %w", err)
	}

	// Write to file
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write peaks file: %w", err)
	}

	return nil
}

// LoadFromFile loads peaks data from a JSON file
func (g *Generator) LoadFromFile(peaksPath string) (*PeaksData, error) {
	data, err := os.ReadFile(peaksPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read peaks file: %w", err)
	}

	var peaks PeaksData
	if err := json.Unmarshal(data, &peaks); err != nil {
		return nil, fmt.Errorf("failed to unmarshal peaks data: %w", err)
	}

	return &peaks, nil
}

type audioInfo struct {
	sampleRate int
	channels   int
	duration   float64
}

// getAudioInfo extracts basic audio information using FFprobe
func (g *Generator) getAudioInfo(audioPath string) (*audioInfo, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=sample_rate,channels,duration",
		"-of", "default=noprint_wrappers=1",
		audioPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	info := &audioInfo{}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.Split(line, "=")
		if len(parts) != 2 {
			continue
		}
		key, value := parts[0], parts[1]
		switch key {
		case "sample_rate":
			info.sampleRate, _ = strconv.Atoi(value)
		case "channels":
			info.channels, _ = strconv.Atoi(value)
		case "duration":
			info.duration, _ = strconv.ParseFloat(value, 64)
		}
	}

	if info.sampleRate == 0 || info.channels == 0 {
		return nil, fmt.Errorf("invalid audio info: sample_rate=%d, channels=%d", info.sampleRate, info.channels)
	}

	return info, nil
}

// extractPeaks uses FFmpeg to extract waveform peaks
func (g *Generator) extractPeaks(audioPath string, samplesPerPixel, sampleRate, channels int) ([]float64, error) {
	// Use FFmpeg to extract raw audio samples
	// We'll downsample to reduce processing time
	targetSampleRate := 8000 // Lower sample rate for faster processing

	cmd := exec.Command("ffmpeg",
		"-i", audioPath,
		"-ac", "1", // Convert to mono
		"-ar", strconv.Itoa(targetSampleRate), // Resample
		"-f", "s16le", // 16-bit signed PCM
		"-acodec", "pcm_s16le",
		"-",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		g.logger.Error("ffmpeg failed", "error", err, "stderr", stderr.String())
		return nil, fmt.Errorf("ffmpeg processing failed: %w", err)
	}

	// Process PCM data to extract peaks
	pcmData := stdout.Bytes()

	// Calculate actual samples per pixel based on the downsampled rate
	adjustedSamplesPerPixel := samplesPerPixel * targetSampleRate / sampleRate
	if adjustedSamplesPerPixel < 1 {
		adjustedSamplesPerPixel = 1
	}

	peaks := g.processPCMData(pcmData, adjustedSamplesPerPixel)
	return peaks, nil
}

// processPCMData converts raw PCM data to normalized peaks
func (g *Generator) processPCMData(pcmData []byte, samplesPerPixel int) []float64 {
	// Process 16-bit signed PCM data
	bytesPerSample := 2
	numSamples := len(pcmData) / bytesPerSample
	if numSamples == 0 {
		return []float64{}
	}

	// Calculate how many peaks we'll generate
	numPeaks := numSamples / samplesPerPixel
	if numPeaks == 0 {
		numPeaks = 1
	}

	peaks := make([]float64, numPeaks)

	// Process each chunk to find peaks
	for i := 0; i < numPeaks; i++ {
		start := i * samplesPerPixel * bytesPerSample
		end := start + samplesPerPixel*bytesPerSample
		if end > len(pcmData) {
			end = len(pcmData)
		}

		// Find max absolute value in this chunk
		maxVal := int16(0)
		for j := start; j < end-1; j += bytesPerSample {
			// Convert 2 bytes to int16 (little endian)
			val := int16(pcmData[j]) | int16(pcmData[j+1])<<8

			// Get absolute value
			if val < 0 {
				val = -val
			}
			if val > maxVal {
				maxVal = val
			}
		}

		// Normalize to 0-1 range
		// int16 max value is 32767
		normalized := float64(maxVal) / 32767.0

		// Scale to 0-255 for 8-bit visualization
		peaks[i] = normalized * 255
	}

	return peaks
}
