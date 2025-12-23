// Package scheduler handles scheduling for recordings and cleanup.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/postprocessor"
	"github.com/oszuidwest/zwfm-audiologger/internal/recorder"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
	cron "github.com/pardnchiu/go-cron"
)

// Scheduler manages scheduled recordings and cleanup tasks.
type Scheduler struct {
	config        *config.Config
	recorder      *recorder.Manager
	postProcessor *postprocessor.Manager
}

// New creates a new scheduler.
func New(cfg *config.Config, rec *recorder.Manager, pp *postprocessor.Manager) *Scheduler {
	return &Scheduler{
		config:        cfg,
		recorder:      rec,
		postProcessor: pp,
	}
}

// Start begins the scheduling using pardnchiu/go-cron.
func (s *Scheduler) Start(ctx context.Context) error {
	// Process any pending recordings on startup
	go func() {
		if err := s.postProcessor.ProcessPendingRecordings(); err != nil {
			slog.Error("failed to process pending recordings", "error", err)
		}
	}()

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

// runAllRecordings records all configured stations.
func (s *Scheduler) runAllRecordings() {
	for name, station := range s.config.Stations {
		go func(stationName string, stationConfig *config.Station) {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic in recording", "station", stationName, "panic", r)
				}
			}()
			s.recordAndProcess(stationName, stationConfig)
		}(name, &station)
	}
}

// runCleanup runs the cleanup with panic recovery.
func (s *Scheduler) runCleanup() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in cleanup", "panic", r)
		}
	}()
	s.cleanupOldRecordings()
}

// recordAndProcess handles recording and post-processing.
func (s *Scheduler) recordAndProcess(name string, station *config.Station) {
	hour := utils.HourlyTimestamp()

	// Do the recording
	s.recorder.Scheduled(name, station)

	// After recording completes, process it to remove commercials if segments were marked
	if err := s.postProcessor.ProcessRecording(name, hour); err != nil {
		slog.Error("failed to post-process recording", "station", name, "error", err)
	}
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
			if info, err := file.Info(); err == nil {
				if info.ModTime().Before(cutoff) {
					path := filepath.Join(dir, file.Name())
					if err := os.Remove(path); err == nil {
						slog.Info("Deleted old recording", "path", path)
					}
				}
			}
		}
	}
}
