// Package scheduler handles scheduling for recordings and cleanup.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
	"github.com/oszuidwest/zwfm-audiologger/internal/recorder"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
	cron "github.com/pardnchiu/go-cron"
)

// Scheduler manages scheduled recordings and cleanup tasks.
type Scheduler struct {
	config   *config.Config
	recorder *recorder.Manager
}

// New creates a new scheduler.
func New(cfg *config.Config, rec *recorder.Manager) *Scheduler {
	return &Scheduler{
		config:   cfg,
		recorder: rec,
	}
}

// Start begins the scheduling using pardnchiu/go-cron.
func (s *Scheduler) Start(ctx context.Context) error {
	// Create scheduler using the global timezone (already set in main)
	scheduler, err := cron.New(cron.Config{
		Location: utils.AppTimezone,
	})
	if err != nil {
		return fmt.Errorf("failed to create scheduler: %w", err)
	}

	// Schedule hourly recordings at minute 0 of every hour
	_, err = scheduler.Add("0 * * * *", s.runAllRecordings, "Hourly recordings")
	if err != nil {
		return fmt.Errorf("failed to schedule hourly recordings: %w", err)
	}

	// Schedule daily cleanup at midnight
	_, err = scheduler.Add("0 0 * * *", s.runCleanup, "Daily cleanup")
	if err != nil {
		return fmt.Errorf("failed to schedule daily cleanup: %w", err)
	}

	// Log scheduled stations
	for name, station := range s.config.Stations {
		slog.Info("Scheduled station for hourly recording", "name", name, "url", station.StreamURL)
	}
	slog.Info("Scheduled daily cleanup", "time", "midnight", "timezone", utils.AppTimezone)

	// If we started mid-hour, immediately record the remaining portion so no
	// broadcast is lost between startup and the first cron trigger at minute 0.
	s.startCatchupRecordings()

	// Start the scheduler
	scheduler.Start()
	slog.Info("Scheduler started. Press Ctrl+C to stop")

	// Wait for context cancellation
	<-ctx.Done()
	slog.Info("Shutting down scheduler")
	shutdownCtx := scheduler.Stop()
	<-shutdownCtx.Done()
	slog.Info("Scheduler stopped")

	return nil
}

// startCatchupRecordings immediately records the remainder of the current hour
// if the service started mid-hour. This closes the gap that would otherwise
// exist between startup and the first cron trigger at minute 0.
func (s *Scheduler) startCatchupRecordings() {
	now := utils.Now()
	elapsedSecs := now.Minute()*60 + now.Second()
	remainingSecs := 3600 - elapsedSecs

	// Skip if too little of the hour remains; a tiny recording is not worth the overhead.
	if remainingSecs < constants.CatchupMinRemainingSecs {
		return
	}

	// Build the timestamp for the start of the current hour in the configured timezone.
	hourStart := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
	timestamp := hourStart.Format(utils.HourlyTimestampFormat)

	slog.Info("Starting catchup recordings for partial hour",
		"timestamp", timestamp,
		"elapsed_secs", elapsedSecs,
		"remaining_secs", remainingSecs)

	for name, station := range s.config.Stations {
		go func(stationName string, stationCfg *config.Station) {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic in catchup recording", "station", stationName, "panic", r, "stack", string(debug.Stack()))
				}
			}()

			// Skip if an audio file for this hour already exists (e.g. previous run captured it).
			// Use ReadDir + IsAudioFile rather than a glob so that side-car files
			// (.meta, .validation.json) do not falsely suppress the catchup.
			dir := filepath.Join(s.config.RecordingsDir, stationName)
			entries, err := os.ReadDir(dir)
			if err != nil && !os.IsNotExist(err) {
				slog.Error("failed to check for existing recordings, skipping catchup",
					"station", stationName, "dir", dir, "error", err)
				return
			}
			for _, e := range entries {
				if !e.IsDir() && strings.HasPrefix(e.Name(), timestamp+".") && utils.IsAudioFile(e.Name()) {
					slog.Info("Catchup skipped, recording already exists",
						"station", stationName, "timestamp", timestamp)
					return
				}
			}

			s.recorder.Catchup(stationName, stationCfg, timestamp, remainingSecs)
		}(name, &station)
	}
}

// runAllRecordings records all configured stations.
func (s *Scheduler) runAllRecordings() {
	for name, station := range s.config.Stations {
		go func(stationName string, stationConfig *config.Station) {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic in recording", "station", stationName, "panic", r, "stack", string(debug.Stack()))
				}
			}()
			s.recorder.Scheduled(stationName, stationConfig)
		}(name, &station)
	}
}

// runCleanup runs the cleanup with panic recovery.
func (s *Scheduler) runCleanup() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in cleanup", "panic", r, "stack", string(debug.Stack()))
		}
	}()
	s.cleanupOldRecordings()
}

// cleanupOldRecordings removes recordings older than configured keep_days.
func (s *Scheduler) cleanupOldRecordings() {
	cutoff := utils.Now().AddDate(0, 0, -s.config.KeepDays)
	slog.Info("Cleaning up old recordings", "cutoff_date", cutoff.Format("2006-01-02"))

	for station := range s.config.Stations {
		dir := filepath.Join(s.config.RecordingsDir, station)
		files, err := os.ReadDir(dir)
		if err != nil {
			slog.Error("failed to read directory", "dir", dir, "error", err)
			continue
		}

		for _, file := range files {
			info, err := file.Info()
			if err != nil {
				slog.Warn("failed to stat file during cleanup", "file", file.Name(), "error", err)
				continue
			}

			if info.ModTime().Before(cutoff) {
				path := filepath.Join(dir, file.Name())
				if err := os.Remove(path); err != nil {
					slog.Error("failed to delete old recording", "path", path, "error", err)
				} else {
					slog.Info("Deleted old recording", "path", path)
				}
			}
		}
	}
}
