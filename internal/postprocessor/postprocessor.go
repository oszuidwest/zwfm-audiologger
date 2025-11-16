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

	// Find the input file with any audio extension
	inputFile, err := utils.FindRecordingFile(m.recordingsDir, station, hour)
	if err != nil {
		return fmt.Errorf("recording file not found for %s hour %s: %w", station, hour, err)
	}

	// Keep original as backup, processed will take its name
	ext := utils.Extension(inputFile)
	originalBackup := utils.RecordingPath(m.recordingsDir, station, hour+".original", ext)
	tempOutput := utils.RecordingPath(m.recordingsDir, station, hour+".temp", ext)

	// Parse the hour to get the recording start time
	recordingStart, err := utils.ParseHourlyTimestamp(hour)
	if err != nil {
		return fmt.Errorf("invalid hour format: %s", hour)
	}

	// Extract each segment to a temporary file
	var segmentFiles []string
	for i, segment := range recording.Segments {
		// Calculate offsets for this segment
		startOffset := segment.StartTime.Sub(recordingStart).Seconds()
		if startOffset < 0 {
			startOffset = 0
		}

		var duration float64
		if !segment.EndTime.IsZero() {
			duration = segment.EndTime.Sub(segment.StartTime).Seconds()
		} else {
			// If no end time, go to the end of the recording
			duration = 3600 - startOffset
		}

		// Skip zero or negative duration segments
		if duration <= 0 {
			slog.Warn("Skipping invalid segment", "station", station, "segment", i+1, "duration", duration)
			continue
		}

		segmentFile := utils.RecordingPath(m.recordingsDir, station, fmt.Sprintf("%s.segment%d", hour, i), ext)
		segmentFiles = append(segmentFiles, segmentFile)

		slog.Info("Extracting segment", "station", station, "segment", i+1, "start_offset", startOffset, "duration", duration)

		cmd := utils.TrimCommand(inputFile, fmt.Sprintf("%.0f", startOffset), fmt.Sprintf("%.0f", duration), segmentFile)
		if err := cmd.Run(); err != nil {
			// Clean up any segment files created so far
			for _, sf := range segmentFiles {
				os.Remove(sf)
			}
			return fmt.Errorf("ffmpeg segment extraction failed for segment %d: %w", i+1, err)
		}
	}

	// If no valid segments were extracted, keep the original
	if len(segmentFiles) == 0 {
		slog.Info("No valid segments extracted, keeping full recording", "station", station, "hour", hour)
		return nil
	}

	// If only one segment, just rename it
	if len(segmentFiles) == 1 {
		// Backup original
		if err := os.Rename(inputFile, originalBackup); err != nil {
			os.Remove(segmentFiles[0])
			return fmt.Errorf("failed to backup original %s: %w", inputFile, err)
		}

		// Move segment to final location
		if err := os.Rename(segmentFiles[0], inputFile); err != nil {
			// Restore original on failure
			os.Rename(originalBackup, inputFile)
			os.Remove(segmentFiles[0])
			return fmt.Errorf("failed to move segment to final location: %w", err)
		}

		slog.Info("Processed recording with single segment", "file", inputFile, "backup", originalBackup)
		return nil
	}

	// Create concat file listing all segments
	concatListFile := utils.RecordingPath(m.recordingsDir, station, hour+".concat.txt", "")
	concatContent := ""
	for _, segmentFile := range segmentFiles {
		// FFmpeg concat requires absolute paths or paths with proper escaping
		concatContent += fmt.Sprintf("file '%s'\n", segmentFile)
	}
	if err := os.WriteFile(concatListFile, []byte(concatContent), constants.FilePermissions); err != nil {
		// Clean up segment files
		for _, sf := range segmentFiles {
			os.Remove(sf)
		}
		return fmt.Errorf("failed to create concat list file: %w", err)
	}

	// Concatenate all segments
	slog.Info("Concatenating segments", "station", station, "count", len(segmentFiles))
	cmd := utils.ConcatCommand(concatListFile, tempOutput)
	if err := cmd.Run(); err != nil {
		// Clean up
		for _, sf := range segmentFiles {
			os.Remove(sf)
		}
		os.Remove(concatListFile)
		os.Remove(tempOutput)
		return fmt.Errorf("ffmpeg concat failed: %w", err)
	}

	// Clean up segment files and concat list
	for _, sf := range segmentFiles {
		os.Remove(sf)
	}
	os.Remove(concatListFile)

	// Rename original to .original backup
	if err := os.Rename(inputFile, originalBackup); err != nil {
		os.Remove(tempOutput)
		return fmt.Errorf("failed to backup original %s: %w", inputFile, err)
	}

	// Rename processed to original name for predictable URLs
	if err := os.Rename(tempOutput, inputFile); err != nil {
		// Try to restore original if rename fails
		if restoreErr := os.Rename(originalBackup, inputFile); restoreErr != nil {
			slog.Error("failed to restore original file", "file", inputFile, "error", restoreErr)
		}
		os.Remove(tempOutput)
		return fmt.Errorf("failed to replace %s with processed version: %w", inputFile, err)
	}

	slog.Info("Processed recording with multiple segments", "file", inputFile, "backup", originalBackup, "segments", len(segmentFiles))

	return nil
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

	data, err := os.ReadFile(recordingFile)
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
