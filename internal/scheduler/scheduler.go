// Package scheduler handles cron-like scheduling for recordings and cleanup
package scheduler

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/flc1125/go-cron/middleware/recovery/v4"
	cron "github.com/flc1125/go-cron/v4"
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
	cron          *cron.Cron
	ctx           context.Context
	cancel        context.CancelFunc
}

// New creates a new scheduler
func New(cfg *config.Config, rec *recorder.Manager, pp *postprocessor.Manager) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	c := cron.New(
		cron.WithContext(ctx),
		cron.WithMiddleware(
			recovery.New(), // Recover from panics in cron jobs
		),
	)
	return &Scheduler{
		config:        cfg,
		recorder:      rec,
		postProcessor: pp,
		cron:          c,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start begins the scheduling using cron
func (s *Scheduler) Start(ctx context.Context) {
	// Process any pending recordings on startup
	go s.postProcessor.PendingRecordings()

	// Schedule hourly recordings for all stations (at the top of every hour)
	for name, station := range s.config.Stations {
		stationName, stationConfig := name, station
		s.cron.AddFunc("0 * * * *", func(ctx context.Context) error {
			s.recordAndProcess(stationName, stationConfig)
			return nil
		})
		log.Printf("Scheduled %s for hourly recording: %s", name, station.StreamURL)
	}

	// Schedule daily cleanup at midnight
	s.cron.AddFunc("0 0 * * *", func(ctx context.Context) error {
		s.cleanupOldRecordings()
		return nil
	})

	log.Println("Scheduler started. Press Ctrl+C to stop.")

	// Start the cron scheduler
	s.cron.Start()

	// Wait for context cancellation
	<-ctx.Done()
	log.Println("Shutting down scheduler...")

	// Cancel the scheduler context and stop
	s.cancel()
	s.cron.Stop()
	log.Println("Scheduler stopped.")
}

// recordAndProcess handles recording and post-processing
func (s *Scheduler) recordAndProcess(name string, station config.Station) {
	hour := utils.HourlyTimestamp()

	// Do the recording
	s.recorder.Scheduled(name, station)

	// After recording completes, process it to remove commercials if segments were marked
	if err := s.postProcessor.Recording(name, hour); err != nil {
		log.Printf("Failed to post-process recording for %s: %v", name, err)
	}
}

// cleanupOldRecordings removes recordings older than configured keep_days
func (s *Scheduler) cleanupOldRecordings() {
	cutoff := time.Now().AddDate(0, 0, -s.config.KeepDays)
	log.Printf("Cleaning up recordings older than %s", cutoff.Format("2006-01-02"))

	for station := range s.config.Stations {
		dir := utils.BuildStationDir(s.config.RecordingsDir, station)
		files, err := os.ReadDir(dir)
		if err != nil {
			utils.LogErrorAndContinue(fmt.Sprintf("read directory %s", dir), err)
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
