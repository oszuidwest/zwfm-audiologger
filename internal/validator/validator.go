package validator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
)

// ValidationJob represents a file to be validated.
type ValidationJob struct {
	FilePath  string
	Station   string
	Timestamp string
}

// Manager handles recording validation.
type Manager struct {
	config  *config.Config
	queue   chan ValidationJob
	alerter *Alerter
	ctx     context.Context
	cancel  context.CancelFunc
}

// New creates a new validation manager.
func New(cfg *config.Config) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		config: cfg,
		queue:  make(chan ValidationJob, constants.ValidationQueueSize),
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize alerter if configured.
	if cfg.Validation != nil && cfg.Validation.Alert != nil && cfg.Validation.Alert.Enabled {
		m.alerter = NewAlerter(cfg.Validation.Alert, cfg.Validation.StationRecipients)
	}

	return m
}

// Start begins the validation worker and scans for unvalidated files.
func (m *Manager) Start(ctx context.Context) error {
	slog.Info("Validator started")

	// Scan for unvalidated files on startup.
	go m.scanUnvalidated()

	// Run worker loop.
	for {
		select {
		case <-ctx.Done():
			slog.Info("Validator shutting down")
			m.cancel()
			return nil
		case <-m.ctx.Done():
			return nil
		case job := <-m.queue:
			m.processJob(job)
		}
	}
}

// Stop gracefully stops the validator.
func (m *Manager) Stop() {
	m.cancel()
}

// Enqueue adds a file to the validation queue (non-blocking).
func (m *Manager) Enqueue(filePath, station, timestamp string) {
	job := ValidationJob{
		FilePath:  filePath,
		Station:   station,
		Timestamp: timestamp,
	}

	select {
	case m.queue <- job:
		slog.Debug("Queued for validation", "file", filePath)
	default:
		slog.Warn("Validation queue full, skipping", "file", filePath)
	}
}

// scanUnvalidated finds recordings without validation files and queues them.
func (m *Manager) scanUnvalidated() {
	slog.Info("Scanning for unvalidated recordings")

	for stationName := range m.config.Stations {
		stationDir := filepath.Join(m.config.RecordingsDir, stationName)

		entries, err := os.ReadDir(stationDir)
		if err != nil {
			if !os.IsNotExist(err) {
				slog.Warn("Failed to scan station directory", "station", stationName, "error", err)
			}
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			name := entry.Name()

			// Skip non-audio files and validation/metadata files.
			if !isAudioFile(name) {
				continue
			}

			// Check if validation file exists.
			baseName := strings.TrimSuffix(name, filepath.Ext(name))
			validationFile := filepath.Join(stationDir, baseName+".validation.json")

			if _, err := os.Stat(validationFile); os.IsNotExist(err) {
				filePath := filepath.Join(stationDir, name)
				m.Enqueue(filePath, stationName, baseName)
			}
		}
	}

	slog.Info("Finished scanning for unvalidated recordings")
}

// isAudioFile checks if a filename has an audio extension.
func isAudioFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp3", ".aac", ".ogg", ".opus", ".flac", ".m4a", ".wav":
		return true
	default:
		return false
	}
}

// processJob validates a single recording.
func (m *Manager) processJob(job ValidationJob) {
	slog.Info("Validating recording", "file", job.FilePath, "station", job.Station)

	result := &ValidationResult{
		Station:     job.Station,
		Timestamp:   job.Timestamp,
		ValidatedAt: time.Now(),
		Valid:       true,
	}

	// Analyze duration.
	duration, err := m.analyzeDuration(m.ctx, job.FilePath)
	if err != nil {
		slog.Error("Duration analysis failed", "file", job.FilePath, "error", err)
		result.Issues = append(result.Issues, fmt.Sprintf("duration analysis failed: %v", err))
		result.Valid = false
	} else {
		result.DurationSecs = duration
		minDuration := float64(m.config.Validation.MinDurationSecs)
		if duration < minDuration {
			result.Issues = append(result.Issues, fmt.Sprintf("duration too short: %.1fs (min: %.1fs)", duration, minDuration))
			result.Valid = false
		}
	}

	// Analyze silence.
	maxSilence, err := m.analyzeSilence(m.ctx, job.FilePath)
	if err != nil {
		slog.Error("Silence analysis failed", "file", job.FilePath, "error", err)
		result.Issues = append(result.Issues, fmt.Sprintf("silence analysis failed: %v", err))
		result.Valid = false
	} else {
		// Calculate silence as percentage of total duration.
		if result.DurationSecs > 0 {
			result.SilencePercent = (maxSilence / result.DurationSecs) * 100
		}
		if maxSilence > m.config.Validation.MaxSilenceSecs {
			result.Issues = append(result.Issues, fmt.Sprintf("silence detected: %.1fs continuous (max: %.1fs)", maxSilence, m.config.Validation.MaxSilenceSecs))
			result.Valid = false
		}
	}

	// Analyze loops.
	loopPercent, err := m.analyzeLoops(m.ctx, job.FilePath)
	if err != nil {
		slog.Error("Loop analysis failed", "file", job.FilePath, "error", err)
		result.Issues = append(result.Issues, fmt.Sprintf("loop analysis failed: %v", err))
		result.Valid = false
	} else {
		result.LoopPercent = loopPercent
		if loopPercent > m.config.Validation.MaxLoopPercent {
			result.Issues = append(result.Issues, fmt.Sprintf("loop detected: %.1f%% (max: %.1f%%)", loopPercent, m.config.Validation.MaxLoopPercent))
			result.Valid = false
		}
	}

	// Save validation result.
	baseName := strings.TrimSuffix(filepath.Base(job.FilePath), filepath.Ext(job.FilePath))
	validationFile := filepath.Join(filepath.Dir(job.FilePath), baseName+".validation.json")

	if err := result.Save(validationFile); err != nil {
		slog.Error("Failed to save validation result", "file", validationFile, "error", err)
	} else {
		slog.Info("Validation result saved", "file", validationFile, "valid", result.Valid)
	}

	// Send alert if invalid and alerter is configured.
	if !result.Valid && m.alerter != nil {
		if err := m.alerter.Send(m.ctx, result); err != nil {
			slog.Error("Failed to send validation alert", "error", err)
		}
	}
}
