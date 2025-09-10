// Package scheduler handles scheduling for recordings and cleanup
package scheduler

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/postprocessor"
	"github.com/oszuidwest/zwfm-audiologger/internal/recorder"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
)

// Scheduler manages scheduled recordings and cleanup tasks
type Scheduler struct {
	config        *config.Config
	recorder      *recorder.Manager
	postProcessor *postprocessor.Manager
}

// New creates a new scheduler
func New(cfg *config.Config, rec *recorder.Manager, pp *postprocessor.Manager) *Scheduler {
	return &Scheduler{
		config:        cfg,
		recorder:      rec,
		postProcessor: pp,
	}
}

// Start begins the scheduling using time-based scheduling
func (s *Scheduler) Start(ctx context.Context) {
	// Process any pending recordings on startup
	go func() {
		if err := s.postProcessor.ProcessPendingRecordings(); err != nil {
			log.Printf("Failed to process pending recordings: %v", err)
		}
	}()

	// Log scheduled stations
	for name, station := range s.config.Stations {
		log.Printf("Scheduled %s for hourly recording: %s", name, station.StreamURL)
	}
	log.Printf("Scheduled daily cleanup at midnight")

	log.Println("Scheduler started. Press Ctrl+C to stop.")

	// Start the scheduling loops
	go s.runHourlyRecordings(ctx)
	go s.runDailyCleanup(ctx)

	// Wait for context cancellation
	<-ctx.Done()
	log.Println("Shutting down scheduler...")
	log.Println("Scheduler stopped.")
}

// runHourlyRecordings runs recordings at the top of every hour
func (s *Scheduler) runHourlyRecordings(ctx context.Context) {
	// Calculate time until next hour
	now := utils.Now()
	nextHour := now.Truncate(time.Hour).Add(time.Hour)
	timeUntilNextHour := nextHour.Sub(now)

	// Wait until the top of the next hour
	select {
	case <-time.After(timeUntilNextHour):
		// Fall through to start the hourly ticker
	case <-ctx.Done():
		return
	}

	// Create a ticker for every hour
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	// Run recordings immediately at the hour, then every hour
	s.runAllRecordings()

	for {
		select {
		case <-ticker.C:
			s.runAllRecordings()
		case <-ctx.Done():
			return
		}
	}
}

// runAllRecordings records all configured stations
func (s *Scheduler) runAllRecordings() {
	for name, station := range s.config.Stations {
		go func(stationName string, stationConfig config.Station) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Panic in recording %s: %v", stationName, r)
				}
			}()
			s.recordAndProcess(stationName, stationConfig)
		}(name, station)
	}
}

// runDailyCleanup runs cleanup at midnight every day
func (s *Scheduler) runDailyCleanup(ctx context.Context) {
	// Calculate time until next midnight
	now := utils.Now()
	nextMidnight := now.Truncate(24 * time.Hour).Add(24 * time.Hour)
	timeUntilMidnight := nextMidnight.Sub(now)

	// Wait until midnight
	select {
	case <-time.After(timeUntilMidnight):
		// Fall through to start the daily ticker
	case <-ctx.Done():
		return
	}

	// Create a ticker for every 24 hours
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Run cleanup immediately at midnight, then every day
	s.runCleanup()

	for {
		select {
		case <-ticker.C:
			s.runCleanup()
		case <-ctx.Done():
			return
		}
	}
}

// runCleanup runs the cleanup with panic recovery
func (s *Scheduler) runCleanup() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in cleanup: %v", r)
		}
	}()
	s.cleanupOldRecordings()
}

// recordAndProcess handles recording and post-processing
func (s *Scheduler) recordAndProcess(name string, station config.Station) {
	hour := utils.HourlyTimestamp()

	// Do the recording
	s.recorder.Scheduled(name, station)

	// After recording completes, process it to remove commercials if segments were marked
	if err := s.postProcessor.ProcessRecording(name, hour); err != nil {
		log.Printf("Failed to post-process recording for %s: %v", name, err)
	}
}

// cleanupOldRecordings removes recordings older than configured keep_days
func (s *Scheduler) cleanupOldRecordings() {
	cutoff := utils.Now().AddDate(0, 0, -s.config.KeepDays)
	log.Printf("Cleaning up recordings older than %s", cutoff.Format("2006-01-02"))

	for station := range s.config.Stations {
		dir := utils.StationDir(s.config.RecordingsDir, station)
		files, err := os.ReadDir(dir)
		if err != nil {
			log.Printf("Failed to read directory %s: %v", dir, err)
			continue
		}

		for _, file := range files {
			if info, err := file.Info(); err == nil {
				if info.ModTime().Before(cutoff) {
					path := filepath.Join(dir, file.Name())
					if err := os.Remove(path); err == nil {
						log.Printf("Deleted old recording: %s", path)
					}
				}
			}
		}
	}
}
