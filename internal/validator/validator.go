package validator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
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
		slog.Debug("queued for validation", "file", filePath)
	default:
		slog.Warn("validation queue full, skipping", "file", filePath)
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
				slog.Warn("failed to scan station directory", "station", stationName, "error", err)
			}
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			name := entry.Name()

			// Skip non-audio files and validation/metadata files.
			if !utils.IsAudioFile(name) {
				continue
			}

			// Check if validation file exists.
			filePath := filepath.Join(stationDir, name)
			validationFile := utils.SidecarPath(filePath, constants.ValidationFileSuffix)

			if _, err := os.Stat(validationFile); os.IsNotExist(err) {
				baseName := filepath.Base(name)
				baseName = baseName[:len(baseName)-len(filepath.Ext(baseName))]
				m.Enqueue(filePath, stationName, baseName)
			}
		}
	}

	slog.Info("Finished scanning for unvalidated recordings")
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
		m.recordAnalysisError(result, "duration", job.FilePath, err)
	} else {
		result.DurationSecs = duration
		minDuration := float64(m.config.Validation.MinDurationSecs)
		if duration < minDuration {
			m.recordIssue(result, fmt.Sprintf("duration too short: %.1fs (min: %.1fs)", duration, minDuration))
		}
	}

	// Analyze silence.
	maxSilence, err := m.analyzeSilence(m.ctx, job.FilePath)
	if err != nil {
		m.recordAnalysisError(result, "silence", job.FilePath, err)
	} else {
		// Calculate silence as percentage of total duration.
		if result.DurationSecs > 0 {
			result.SilencePercent = (maxSilence / result.DurationSecs) * 100
		}
		if maxSilence > m.config.Validation.MaxSilenceSecs {
			m.recordIssue(result, fmt.Sprintf("silence detected: %.1fs continuous (max: %.1fs)", maxSilence, m.config.Validation.MaxSilenceSecs))
		}
	}

	// Analyze loops.
	loopPercent, err := m.analyzeLoops(m.ctx, job.FilePath)
	if err != nil {
		m.recordAnalysisError(result, "loop", job.FilePath, err)
	} else {
		result.LoopPercent = loopPercent
		if loopPercent > m.config.Validation.MaxLoopPercent {
			m.recordIssue(result, fmt.Sprintf("loop detected: %.1f%% (max: %.1f%%)", loopPercent, m.config.Validation.MaxLoopPercent))
		}
	}

	// Save validation result.
	validationFile := utils.SidecarPath(job.FilePath, constants.ValidationFileSuffix)

	if err := result.Save(validationFile); err != nil {
		slog.Error("failed to save validation result", "file", validationFile, "error", err)
	} else {
		slog.Info("Validation result saved", "file", validationFile, "valid", result.Valid)
	}

	// Send alert if invalid and alerter is configured.
	if !result.Valid && m.alerter != nil {
		if err := m.alerter.Send(m.ctx, result); err != nil {
			slog.Error("failed to send validation alert", "error", err)
		}
	}
}

// recordAnalysisError logs an analysis error and records it in the result.
func (m *Manager) recordAnalysisError(result *ValidationResult, analysisName, filePath string, err error) {
	slog.Error(fmt.Sprintf("failed to analyze %s", analysisName), "file", filePath, "error", err)
	result.Issues = append(result.Issues, fmt.Sprintf("%s analysis failed: %v", analysisName, err))
	result.Valid = false
}

// recordIssue adds an issue to the result and marks it as invalid.
func (m *Manager) recordIssue(result *ValidationResult, issue string) {
	result.Issues = append(result.Issues, issue)
	result.Valid = false
}
