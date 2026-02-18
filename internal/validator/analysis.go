package validator

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
)

// probeFormat holds the ffprobe format output structure.
type probeFormat struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// analyzeDuration uses ffprobe to get the duration of a recording in seconds.
func (m *Manager) analyzeDuration(ctx context.Context, file string) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, constants.ValidationAnalysisTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffprobe", //nolint:gosec // G204: args are from internal file paths
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		file,
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w", err)
	}

	var result probeFormat
	if err := json.Unmarshal(output, &result); err != nil {
		return 0, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	duration, err := strconv.ParseFloat(result.Format.Duration, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}

	return duration, nil
}

// silenceRegex matches FFmpeg silencedetect output lines.
var silenceRegex = regexp.MustCompile(`silence_(start|end|duration):\s*([\d.]+)`)

// analyzeSilence detects silence periods in the recording and returns the maximum
// continuous silence duration in seconds.
func (m *Manager) analyzeSilence(ctx context.Context, file string) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, constants.ValidationAnalysisTimeout)
	defer cancel()

	threshold := fmt.Sprintf("%ddB", int(m.config.Validation.SilenceThresholdDB))
	minDuration := fmt.Sprintf("%.1f", m.config.Validation.MaxSilenceSecs)

	cmd := exec.CommandContext(ctx, "ffmpeg", //nolint:gosec // G204: args are from internal file paths
		"-i", file,
		"-af", fmt.Sprintf("silencedetect=noise=%s:d=%s", threshold, minDuration),
		"-f", "null",
		"-",
	)

	// silencedetect outputs to stderr.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	_ = cmd.Run() // Ignore error; ffmpeg returns non-zero for -f null.

	var maxSilence float64
	scanner := bufio.NewScanner(&stderr)
	for scanner.Scan() {
		line := scanner.Text()
		matches := silenceRegex.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) >= 3 && match[1] == "duration" {
				duration, err := strconv.ParseFloat(match[2], 64)
				if err == nil && duration > maxSilence {
					maxSilence = duration
				}
			}
		}
	}

	return maxSilence, nil
}

// analyzeLoops detects looping/repeating content by analyzing audio energy patterns.
// It returns the estimated percentage of content that appears to be looped.
func (m *Manager) analyzeLoops(ctx context.Context, file string) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, constants.ValidationAnalysisTimeout)
	defer cancel()

	// Use astats to get per-second RMS energy values.
	cmd := exec.CommandContext(ctx, "ffmpeg", //nolint:gosec // G204: args are from internal file paths
		"-i", file,
		"-af", "astats=metadata=1:reset=1,ametadata=print:file=-",
		"-f", "null",
		"-",
	)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	_ = cmd.Run() // Ignore error; ffmpeg returns non-zero for -f null.

	// Parse RMS values from astats output.
	rmsValues := parseRMSValues(stdout.String())
	if len(rmsValues) < 60 {
		// Not enough data for meaningful analysis.
		return 0, nil
	}

	// Detect loops using autocorrelation of energy patterns.
	loopPercent := detectLoopsViaAutocorrelation(rmsValues)

	return loopPercent, nil
}

// rmsRegex matches astats RMS_level output.
var rmsRegex = regexp.MustCompile(`RMS_level=(-?[\d.]+|inf|-inf)`)

// parseRMSValues extracts RMS level values from astats output.
func parseRMSValues(output string) []float64 {
	var values []float64
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		match := rmsRegex.FindStringSubmatch(line)
		if len(match) >= 2 {
			if match[1] == "-inf" || match[1] == "inf" {
				values = append(values, -100.0) // Use -100 dB for silence.
			} else if rms, err := strconv.ParseFloat(match[1], 64); err == nil {
				values = append(values, rms)
			}
		}
	}
	return values
}

// detectLoopsViaAutocorrelation analyzes energy patterns to detect repeating content.
// It looks for strong periodic correlations that would indicate looped audio.
func detectLoopsViaAutocorrelation(rmsValues []float64) float64 {
	n := len(rmsValues)
	if n < 60 {
		return 0
	}

	// Normalize values.
	mean := 0.0
	for _, v := range rmsValues {
		mean += v
	}
	mean /= float64(n)

	variance := 0.0
	normalized := make([]float64, n)
	for i, v := range rmsValues {
		normalized[i] = v - mean
		variance += normalized[i] * normalized[i]
	}
	variance /= float64(n)

	if variance < 0.0001 {
		// Near-constant signal, treat as potential loop.
		return 100.0
	}

	// Calculate autocorrelation for different lag values.
	// Look for lags between 10 seconds and half the recording.
	minLag := 10
	maxLag := n / 2
	if maxLag > 300 {
		maxLag = 300 // Cap at 5 minutes to limit computation.
	}

	var highCorrelationCount int
	var totalChecks int

	for lag := minLag; lag < maxLag; lag++ {
		correlation := 0.0
		count := 0
		for i := 0; i < n-lag; i++ {
			correlation += normalized[i] * normalized[i+lag]
			count++
		}
		if count > 0 {
			correlation /= float64(count) * variance
			totalChecks++

			// High correlation suggests repeating pattern.
			if correlation > 0.85 {
				highCorrelationCount++
			}
		}
	}

	if totalChecks == 0 {
		return 0
	}

	// Calculate percentage of lags with high correlation.
	loopPercent := float64(highCorrelationCount) / float64(totalChecks) * 100

	// Round to one decimal place.
	return math.Round(loopPercent*10) / 10
}
