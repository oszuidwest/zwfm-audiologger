package validator

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
)

// probeFormat holds the ffprobe format output structure.
type probeFormat struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// withAnalysisTimeout creates a context with the standard analysis timeout.
func withAnalysisTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, constants.ValidationAnalysisTimeout)
}

// analyzeDuration uses ffprobe to get the duration of a recording in seconds.
func (m *Manager) analyzeDuration(ctx context.Context, file string) (float64, error) {
	ctx, cancel := withAnalysisTimeout(ctx)
	defer cancel()

	cmd := utils.ProbeCommand(ctx, file)
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
	ctx, cancel := withAnalysisTimeout(ctx)
	defer cancel()

	thresholdDB := int(m.config.Validation.SilenceThresholdDB)
	minDuration := m.config.Validation.MaxSilenceSecs

	cmd := utils.SilenceDetectCommand(ctx, file, thresholdDB, minDuration)

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
	ctx, cancel := withAnalysisTimeout(ctx)
	defer cancel()

	cmd := utils.AudioStatsCommand(ctx, file)

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
		return 100.0
	}

	minLag := 10
	maxLag := min(n/2, 300)

	highCorrelationCount := 0
	totalChecks := maxLag - minLag

	for lag := minLag; lag < maxLag; lag++ {
		correlation := 0.0
		count := n - lag
		for i := 0; i < count; i++ {
			correlation += normalized[i] * normalized[i+lag]
		}
		correlation /= float64(count) * variance

		if correlation > 0.85 {
			highCorrelationCount++
		}
	}

	if totalChecks == 0 {
		return 0
	}

	loopPercent := float64(highCorrelationCount) / float64(totalChecks) * 100
	return math.Round(loopPercent*10) / 10
}
