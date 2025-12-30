// Package scheduler handles scheduling for recordings and cleanup.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/oszuidwest/zwfm-audiologger/internal/alerting"
	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
	"github.com/oszuidwest/zwfm-audiologger/internal/ffmpeg"
	"github.com/oszuidwest/zwfm-audiologger/internal/recorder"
	"github.com/oszuidwest/zwfm-audiologger/internal/timeutil"
	cron "github.com/pardnchiu/go-cron"
)

// withPanicRecovery wraps a function with panic recovery logging.
func withPanicRecovery(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic recovered", "context", name, "panic", r)
		}
	}()
	fn()
}

// Scheduler manages scheduled recordings and cleanup tasks.
type Scheduler struct {
	config   *config.Config
	recorder *recorder.Manager
	alerter  *alerting.Manager
	ctx      context.Context
}

// New creates a new scheduler.
func New(cfg *config.Config, rec *recorder.Manager, alerter *alerting.Manager) *Scheduler {
	return &Scheduler{
		config:   cfg,
		recorder: rec,
		alerter:  alerter,
	}
}

// Start begins the scheduling using pardnchiu/go-cron.
func (s *Scheduler) Start(ctx context.Context) error {
	// Store context for use in scheduled tasks
	s.ctx = ctx

	// Create scheduler using the global timezone (already set in main)
	scheduler, err := cron.New(cron.Config{
		Location: timeutil.Timezone(),
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

	// Schedule disk space check every 15 minutes (at :00, :15, :30, :45)
	_, err = scheduler.Add("*/15 * * * *", s.checkDiskSpace, "Disk space check")
	if err != nil {
		return fmt.Errorf("failed to schedule disk space check: %w", err)
	}

	// Log scheduled stations
	for name, station := range s.config.Stations {
		slog.Info("Scheduled station for hourly recording", "name", name, "url", station.StreamURL)
	}
	slog.Info("Scheduled daily cleanup", "time", "midnight", "timezone", timeutil.Timezone())
	slog.Info("Scheduled disk space check", "interval", "every 15 minutes")

	// Check if we should record immediately (mid-hour start)
	s.checkMidHourStart()

	// Run initial disk space check
	go s.checkDiskSpace()

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
// This spawns a goroutine to manage all recordings so it doesn't block the scheduler,
// but properly waits for all station recordings to complete before logging.
func (s *Scheduler) runAllRecordings() {
	// Run the entire recording batch in a goroutine to not block the scheduler
	go func() {
		var wg sync.WaitGroup

		for name, station := range s.config.Stations {
			station := station // Create local copy for pointer safety
			wg.Go(func() {
				withPanicRecovery("recording station: "+name, func() {
					// Record and get the final file path
					finalFile := s.recorder.Scheduled(s.ctx, name, &station)

					// Check recording duration if file was created successfully
					if finalFile != "" {
						s.checkRecordingDuration(name, finalFile, constants.HourlyRecordingDurationSeconds)
					}
				})
			})
		}

		// Wait for all recordings to complete
		wg.Wait()
		slog.Debug("All hourly recordings completed")
	}()
}

// checkMidHourStart checks if the app started mid-hour and initiates immediate recording if needed.
func (s *Scheduler) checkMidHourStart() {
	now := timeutil.LocalNow()

	// Check if we're not exactly at the top of the hour
	if now.Minute() == 0 && now.Second() == 0 {
		slog.Debug("Starting at top of hour, skipping immediate recording")
		return
	}

	// Calculate remaining seconds until next hour
	nextHour := now.Truncate(time.Hour).Add(time.Hour)
	remainingSeconds := int(nextHour.Sub(now).Seconds())

	// Only record if there's meaningful time left (at least minimum threshold)
	if remainingSeconds < constants.MinimumRecordingSeconds {
		slog.Info("Skipping mid-hour recording, insufficient time remaining",
			"remaining_seconds", remainingSeconds,
			"minimum_required", constants.MinimumRecordingSeconds,
			"next_full_recording", nextHour.Format("15:04"))
		return
	}

	// Get the timestamp for the current hour (for file naming)
	// This ensures the file is named for the hour it represents (e.g., 2024-01-15-14.mp3)
	currentHour := now.Truncate(time.Hour)
	timestamp := currentHour.Format(timeutil.HourlyTimestampFormat)

	slog.Info("Starting mid-hour recording for remainder of current hour",
		"current_time", now.Format("15:04:05"),
		"hour_timestamp", timestamp,
		"remaining_seconds", remainingSeconds,
		"next_full_recording", nextHour.Format("15:04"))

	// Start immediate recording in goroutine
	go s.runImmediateRecording(timestamp, remainingSeconds)
}

// runImmediateRecording records all stations for the specified duration.
// This is used for mid-hour recordings when the app starts mid-hour.
// It properly waits for all station recordings to complete before logging.
func (s *Scheduler) runImmediateRecording(timestamp string, durationSeconds int) {
	var wg sync.WaitGroup

	for name, station := range s.config.Stations {
		station := station // Create local copy for pointer safety
		wg.Go(func() {
			withPanicRecovery("immediate recording station: "+name, func() {
				slog.Info("Starting immediate recording",
					"station", name,
					"duration_seconds", durationSeconds,
					"hour_timestamp", timestamp)

				// Record and get the final file path
				finalFile := s.recorder.ScheduledWithDuration(s.ctx, name, &station, timestamp, durationSeconds)

				// Check recording duration if file was created successfully
				if finalFile != "" {
					s.checkRecordingDuration(name, finalFile, durationSeconds)
				}
			})
		})
	}

	// Wait for all immediate recordings to complete
	wg.Wait()
	slog.Debug("All immediate recordings completed", "timestamp", timestamp)
}

// runCleanup runs the cleanup with panic recovery.
func (s *Scheduler) runCleanup() {
	withPanicRecovery("cleanup", s.cleanupOldRecordings)
}

// cleanupOldRecordings removes recordings older than configured keep_days.
func (s *Scheduler) cleanupOldRecordings() {
	cutoff := timeutil.LocalNow().AddDate(0, 0, -s.config.KeepDays)
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

// checkRecordingDuration verifies that a recording meets the minimum duration threshold.
// If the duration is below the configured threshold, an incomplete recording alert is sent.
func (s *Scheduler) checkRecordingDuration(stationName, filePath string, expectedDurationSeconds int) {
	// Skip duration check if alerter is not configured
	if s.alerter == nil {
		return
	}

	// Get actual duration using ffprobe
	// Use a 1-minute timeout for duration probing (should be very quick)
	probeCtx, probeCancel := context.WithTimeout(s.ctx, 1*time.Minute)
	actualDuration, err := ffmpeg.ProbeDuration(probeCtx, filePath)
	probeCancel()
	if err != nil {
		slog.Warn("failed to probe recording duration",
			"station", stationName,
			"file", filePath,
			"error", err)
		return
	}

	threshold := s.config.Alerting.IncompleteThresholdSeconds

	// Check if duration is below threshold
	if int(actualDuration) < threshold {
		// Extract hour from filename for the alert
		hour := filepath.Base(filePath)

		reason := fmt.Sprintf("Recording duration (%.0fs) is below threshold (%ds). Expected duration: %ds",
			actualDuration, threshold, expectedDurationSeconds)

		slog.Warn("incomplete recording detected",
			"station", stationName,
			"file", filePath,
			"actual_duration", actualDuration,
			"threshold", threshold,
			"expected_duration", expectedDurationSeconds)

		event := alerting.NewEvent(alerting.EventRecordingIncomplete, stationName, reason).
			WithDetail("hour", hour)
		s.alerter.Alert(s.ctx, event)
	} else {
		slog.Debug("recording duration verified",
			"station", stationName,
			"file", filePath,
			"duration", actualDuration,
			"threshold", threshold)
	}
}

// checkDiskSpace checks the available disk space and sends an alert if it's below the threshold.
func (s *Scheduler) checkDiskSpace() {
	withPanicRecovery("disk space check", s.checkDiskSpaceImpl)
}

// checkDiskSpaceImpl contains the actual disk space checking logic.
func (s *Scheduler) checkDiskSpaceImpl() {
	// Skip if alerter is not configured
	if s.alerter == nil {
		return
	}

	diskInfo, err := getDiskSpace(s.config.RecordingsDir)
	if err != nil {
		slog.Error("failed to get disk space",
			"path", s.config.RecordingsDir,
			"error", err)
		return
	}

	threshold := float64(s.config.Alerting.DiskThresholdPercent)

	slog.Debug("disk space check",
		"path", s.config.RecordingsDir,
		"free_percent", diskInfo.FreePercent,
		"threshold_percent", threshold,
		"available_bytes", diskInfo.AvailableBytes,
		"total_bytes", diskInfo.TotalBytes)

	// Alert if free space is below threshold
	if diskInfo.FreePercent < threshold {
		slog.Warn("disk space low",
			"path", s.config.RecordingsDir,
			"free_percent", diskInfo.FreePercent,
			"threshold_percent", threshold)

		// Convert threshold percentage to bytes for the alert
		thresholdBytes := uint64(threshold / 100 * float64(diskInfo.TotalBytes))
		event := alerting.NewEvent(alerting.EventDiskSpaceLow, "", "Disk space is running low").
			WithDetail("path", s.config.RecordingsDir).
			WithDetail("available_bytes", humanize.IBytes(diskInfo.AvailableBytes)).
			WithDetail("threshold_bytes", humanize.IBytes(thresholdBytes))
		s.alerter.Alert(s.ctx, event)
	}
}
