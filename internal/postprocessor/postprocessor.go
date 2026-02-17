// Package postprocessor handles trimming commercials from recordings.
package postprocessor

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
)

// Segment represents a single program segment with start and end times.
type Segment struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

// Recording represents program segments within an hour.
type Recording struct {
	Segments []Segment `json:"segments"`
	Station  string    `json:"station"`
	Hour     string    `json:"hour"` // Format: "2006-01-02-15"
}

// Manager handles post-processing of recordings.
type Manager struct {
	recordingsDir string
	stations      map[string]struct {
		bufferOffset int
	}
	mu sync.RWMutex
}

// New creates a new post-processor manager.
func New(recordingsDir string, stations map[string]int) *Manager {
	stationMap := make(map[string]struct{ bufferOffset int })
	for name, offset := range stations {
		stationMap[name] = struct{ bufferOffset int }{bufferOffset: offset}
	}
	return &Manager{
		recordingsDir: recordingsDir,
		stations:      stationMap,
	}
}

// MarkType represents the type of program mark.
type MarkType int

const (
	// MarkStart indicates program start (commercials end).
	MarkStart MarkType = iota
	// MarkEnd indicates program end (commercials start).
	MarkEnd
)

// MarkProgram marks when a program starts or ends.
func (m *Manager) MarkProgram(station string, markType MarkType) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := utils.Now()

	// Apply buffer offset if configured for this station
	adjustedTime := now
	if stationInfo, exists := m.stations[station]; exists && stationInfo.bufferOffset > 0 {
		adjustedTime = now.Add(-time.Duration(stationInfo.bufferOffset) * time.Second)
		slog.Info("Applied buffer offset", "station", station, "offset_seconds", stationInfo.bufferOffset, "original_time", now.Format("15:04:05"), "adjusted_time", adjustedTime.Format("15:04:05"))
	}

	hour := adjustedTime.Format(utils.HourlyTimestampFormat)

	// Load existing recording or create new one
	recording := m.loadRecording(station, hour)
	if recording == nil {
		recording = &Recording{
			Segments: []Segment{},
			Station:  station,
			Hour:     hour,
		}
	}

	switch markType {
	case MarkStart:
		// Add a new segment with start time
		recording.Segments = append(recording.Segments, Segment{
			StartTime: adjustedTime,
		})
		slog.Info("Program segment started", "station", station, "time", adjustedTime.Format("15:04:05"), "segment", len(recording.Segments))
		m.saveRecording(recording)

	case MarkEnd:
		// Find the last incomplete segment and set its end time
		if len(recording.Segments) > 0 {
			lastIdx := len(recording.Segments) - 1
			if recording.Segments[lastIdx].EndTime.IsZero() {
				recording.Segments[lastIdx].EndTime = adjustedTime
				slog.Info("Program segment ended", "station", station, "time", adjustedTime.Format("15:04:05"), "segment", lastIdx+1)
				m.saveRecording(recording)
			} else {
				slog.Warn("No open segment to end", "station", station)
			}
		} else {
			slog.Warn("No segments to end", "station", station)
		}
	}
}

// ProcessRecording processes a completed hourly recording to extract and concatenate program segments.
func (m *Manager) ProcessRecording(station, hour string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	recording := m.loadRecording(station, hour)
	if recording == nil || len(recording.Segments) == 0 {
		slog.Info("No segments found, keeping full recording", "station", station, "hour", hour)
		return nil
	}

	inputFile, err := utils.FindRecordingFile(m.recordingsDir, station, hour)
	if err != nil {
		return fmt.Errorf("recording file not found for %s hour %s: %w", station, hour, err)
	}

	recordingStart, err := utils.ParseHourlyTimestamp(hour)
	if err != nil {
		return fmt.Errorf("invalid hour format: %s", hour)
	}

	ext := utils.Extension(inputFile)
	segmentFiles, err := m.extractSegments(inputFile, recording, station, hour, ext, recordingStart)
	if err != nil {
		return err
	}

	if len(segmentFiles) == 0 {
		slog.Info("No valid segments extracted, keeping full recording", "station", station, "hour", hour)
		return nil
	}

	originalBackup := utils.RecordingPath(m.recordingsDir, station, hour+".original", ext)

	if len(segmentFiles) == 1 {
		return m.processSingleSegment(inputFile, originalBackup, segmentFiles[0])
	}

	tempOutput := utils.RecordingPath(m.recordingsDir, station, hour+".temp", ext)
	concatListFile := utils.RecordingPath(m.recordingsDir, station, hour+".concat.txt", "")

	return m.processMultipleSegments(inputFile, originalBackup, tempOutput, concatListFile, segmentFiles, station)
}

// extractSegments extracts each program segment to a temporary file.
func (m *Manager) extractSegments(inputFile string, recording *Recording, station, hour, ext string, recordingStart time.Time) ([]string, error) {
	var segmentFiles []string

	for i := range recording.Segments {
		segment := &recording.Segments[i]
		startOffset, duration := calculateSegmentTiming(segment, recordingStart)

		if duration <= 0 {
			slog.Warn("Skipping invalid segment", "station", station, "segment", i+1, "duration", duration)
			continue
		}

		segmentFile := utils.RecordingPath(m.recordingsDir, station, fmt.Sprintf("%s.segment%d", hour, i), ext)
		segmentFiles = append(segmentFiles, segmentFile)

		slog.Info("Extracting segment", "station", station, "segment", i+1, "start_offset", startOffset, "duration", duration)

		cmd := utils.TrimCommand(inputFile, fmt.Sprintf("%.0f", startOffset), fmt.Sprintf("%.0f", duration), segmentFile)
		if err := cmd.Run(); err != nil {
			cleanupFiles(segmentFiles)
			return nil, fmt.Errorf("ffmpeg segment extraction failed for segment %d: %w", i+1, err)
		}
	}

	return segmentFiles, nil
}

// calculateSegmentTiming calculates start offset and duration for a segment.
func calculateSegmentTiming(segment *Segment, recordingStart time.Time) (startOffset, duration float64) {
	startOffset = segment.StartTime.Sub(recordingStart).Seconds()
	if startOffset < 0 {
		startOffset = 0
	}

	if !segment.EndTime.IsZero() {
		duration = segment.EndTime.Sub(segment.StartTime).Seconds()
	} else {
		duration = 3600 - startOffset
	}

	return startOffset, duration
}

// processSingleSegment handles the case of a single extracted segment.
func (m *Manager) processSingleSegment(inputFile, originalBackup, segmentFile string) error {
	if err := os.Rename(inputFile, originalBackup); err != nil {
		cleanupFiles([]string{segmentFile})
		return fmt.Errorf("failed to backup original %s: %w", inputFile, err)
	}

	if err := os.Rename(segmentFile, inputFile); err != nil {
		if restoreErr := os.Rename(originalBackup, inputFile); restoreErr != nil {
			slog.Error("failed to restore original file", "file", inputFile, "error", restoreErr)
		}
		cleanupFiles([]string{segmentFile})
		return fmt.Errorf("failed to move segment to final location: %w", err)
	}

	slog.Info("Processed recording with single segment", "file", inputFile, "backup", originalBackup)
	return nil
}

// processMultipleSegments handles concatenating multiple segments.
func (m *Manager) processMultipleSegments(inputFile, originalBackup, tempOutput, concatListFile string, segmentFiles []string, station string) error {
	concatContent := buildConcatContent(segmentFiles)
	if err := os.WriteFile(concatListFile, []byte(concatContent), constants.FilePermissions); err != nil {
		cleanupFiles(segmentFiles)
		return fmt.Errorf("failed to create concat list file: %w", err)
	}

	slog.Info("Concatenating segments", "station", station, "count", len(segmentFiles))
	cmd := utils.ConcatCommand(concatListFile, tempOutput)
	if err := cmd.Run(); err != nil {
		cleanupFiles(segmentFiles)
		cleanupFiles([]string{concatListFile, tempOutput})
		return fmt.Errorf("ffmpeg concat failed: %w", err)
	}

	cleanupFiles(segmentFiles)
	cleanupFiles([]string{concatListFile})

	return m.finalizeProcessedFile(inputFile, originalBackup, tempOutput, len(segmentFiles))
}

// finalizeProcessedFile moves the processed file to its final location.
func (m *Manager) finalizeProcessedFile(inputFile, originalBackup, tempOutput string, segmentCount int) error {
	if err := os.Rename(inputFile, originalBackup); err != nil {
		cleanupFiles([]string{tempOutput})
		return fmt.Errorf("failed to backup original %s: %w", inputFile, err)
	}

	if err := os.Rename(tempOutput, inputFile); err != nil {
		if restoreErr := os.Rename(originalBackup, inputFile); restoreErr != nil {
			slog.Error("failed to restore original file", "file", inputFile, "error", restoreErr)
		}
		cleanupFiles([]string{tempOutput})
		return fmt.Errorf("failed to replace %s with processed version: %w", inputFile, err)
	}

	slog.Info("Processed recording with multiple segments", "file", inputFile, "backup", originalBackup, "segments", segmentCount)
	return nil
}

// buildConcatContent builds the FFmpeg concat file content.
func buildConcatContent(segmentFiles []string) string {
	var content strings.Builder
	for _, segmentFile := range segmentFiles {
		fmt.Fprintf(&content, "file '%s'\n", segmentFile)
	}
	return content.String()
}

// cleanupFiles removes temporary files, logging warnings on failure.
func cleanupFiles(files []string) {
	for _, f := range files {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to remove temporary file", "file", f, "error", err)
		}
	}
}

// saveRecording saves recording information to a JSON file.
// Callers must ensure recording is not nil before calling this function.
func (m *Manager) saveRecording(recording *Recording) {
	// Save to a JSON file alongside the recording
	recordingFile := utils.RecordingPath(m.recordingsDir, recording.Station, recording.Hour, ".recording.json")

	data, err := json.MarshalIndent(recording, "", "  ")
	if err != nil {
		slog.Error("failed to marshal recording", "station", recording.Station, "error", err)
		return
	}

	if err := os.WriteFile(recordingFile, data, constants.FilePermissions); err != nil {
		slog.Error("failed to save recording info", "file", recordingFile, "error", err)
		return
	}

	slog.Info("Saved recording info", "file", recordingFile)
}

// loadRecording loads recording information from disk.
func (m *Manager) loadRecording(station, hour string) *Recording {
	recordingFile := utils.RecordingPath(m.recordingsDir, station, hour, ".recording.json")

	data, err := os.ReadFile(recordingFile) //nolint:gosec // Path is constructed from trusted internal values
	if err != nil {
		return nil // No recording file
	}

	var recording Recording
	if err := json.Unmarshal(data, &recording); err != nil {
		return nil
	}

	return &recording
}

// ProcessPendingRecordings processes any recordings that have recording info but haven't been processed yet.
func (m *Manager) ProcessPendingRecordings() error {
	// Look for .recording.json files without corresponding _processed.mp3 files
	stations, err := os.ReadDir(m.recordingsDir)
	if err != nil {
		return err
	}

	for _, stationDir := range stations {
		if !stationDir.IsDir() {
			continue
		}

		stationPath := filepath.Join(m.recordingsDir, stationDir.Name())
		files, err := os.ReadDir(stationPath)
		if err != nil {
			continue
		}

		for _, file := range files {
			if !strings.HasSuffix(file.Name(), ".recording.json") {
				continue
			}

			hour := strings.TrimSuffix(file.Name(), ".recording.json")

			// Check if we have an original backup (means it was already processed)
			originalFiles, err := filepath.Glob(filepath.Join(stationPath, hour+".original.*"))
			if err != nil {
				slog.Error("invalid glob pattern", "hour", hour, "error", err)
				continue
			}
			if len(originalFiles) > 0 {
				continue // Already processed
			}

			// Find the actual recording file
			if _, err := utils.FindRecordingFile(m.recordingsDir, stationDir.Name(), hour); err != nil {
				continue // No recording file found
			}

			// Process recording (it will load the info directly)
			slog.Info("Processing pending recording", "station", stationDir.Name(), "hour", hour)
			if err := m.ProcessRecording(stationDir.Name(), hour); err != nil {
				slog.Error("failed to process pending recording", "station", stationDir.Name(), "hour", hour, "error", err)
			}
		}
	}

	return nil
}
