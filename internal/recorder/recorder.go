package recorder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/logger"
	"github.com/oszuidwest/zwfm-audiologger/internal/metadata"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// Recorder handles audio stream recording
// Optimized struct field ordering for Go 1.24+ memory alignment
type Recorder struct {
	cron     *cron.Cron
	config   *config.Config
	logger   *logger.Logger
	metadata *metadata.Fetcher
}

// New creates a new Recorder instance
func New(cfg *config.Config, log *logger.Logger) *Recorder {
	return &Recorder{
		config:   cfg,
		logger:   log,
		cron:     cron.New(),
		metadata: metadata.New(log),
	}
}

// StartCron starts the cron scheduler for hourly recordings
func (r *Recorder) StartCron(ctx context.Context) error {
	// Schedule recording to run every hour at minute 0
	_, err := r.cron.AddFunc("0 * * * *", func() {
		if err := r.RecordAll(ctx); err != nil {
			r.logger.Error("Scheduled recording failed: ", err)
		}
	})
	if err != nil {
		return fmt.Errorf("failed to add cron job: %w", err)
	}
	
	r.cron.Start()
	r.logger.Info("Cron scheduler started - recordings will run every hour")
	
	// Wait for context cancellation
	<-ctx.Done()
	r.cron.Stop()
	r.logger.Info("Cron scheduler stopped")
	return nil
}

// RecordAll records all configured streams
func (r *Recorder) RecordAll(ctx context.Context) error {
	timestamp := utils.FormatTimestamp(time.Now())
	
	// Ensure recording directory exists
	if err := utils.EnsureDir(r.config.RecordingDir); err != nil {
		return fmt.Errorf("failed to create recording directory: %w", err)
	}

	var wg sync.WaitGroup
	
	for streamName, stream := range r.config.Streams {
		wg.Add(1)
		go func(name string, s config.Stream) {
			defer wg.Done()
			
			if err := r.recordStream(ctx, name, s, timestamp); err != nil {
				r.logger.WithStation(name).Error("Recording failed: ", err)
			}
		}(streamName, stream)
	}
	
	wg.Wait()
	return nil
}

// recordStream records a single stream
func (r *Recorder) recordStream(ctx context.Context, streamName string, stream config.Stream, timestamp string) error {
	log := r.logger.WithStation(streamName)
	
	// Create stream directory
	streamDir := utils.StreamDir(r.config.RecordingDir, streamName)
	if err := utils.EnsureDir(streamDir); err != nil {
		return fmt.Errorf("failed to create stream directory: %w", err)
	}

	// Start cleanup in background
	go r.cleanupOldFiles(streamName, streamDir)

	// Define output file path
	outputFile := utils.RecordingPath(r.config.RecordingDir, streamName, timestamp)

	// Check if output file already exists (simple duplicate prevention)
	if utils.FileExists(outputFile) {
		log.Warn("Recording already exists for this hour: ", timestamp)
		return nil
	}

	// Start metadata fetch in background
	go r.metadata.Fetch(streamName, stream, streamDir, timestamp)

	// Start recording
	log.Info("Starting recording: ", timestamp)
	
	if err := r.startFFmpeg(ctx, stream.URL, outputFile, stream.RecordDuration); err != nil {
		return fmt.Errorf("ffmpeg recording failed: %w", err)
	}

	log.Info("Recording completed: ", timestamp)
	return nil
}

// startFFmpeg starts the FFmpeg recording process using ffmpeg-go
func (r *Recorder) startFFmpeg(ctx context.Context, streamURL, outputFile string, duration time.Duration) error {
	// Create ffmpeg command with reconnection settings for robust streaming
	stream := ffmpeg.Input(streamURL, ffmpeg.KwArgs{
		"reconnect":               "1",
		"reconnect_at_eof":        "1", 
		"reconnect_streamed":      "1",
		"reconnect_delay_max":     "300",
		"reconnect_on_http_error": "404,500,503",
		"rw_timeout":              "10000000",
		"t":                       fmt.Sprintf("%.0f", duration.Seconds()),
	})

	// Output with codec copy for efficiency
	err := stream.Output(outputFile, ffmpeg.KwArgs{
		"c":    "copy",
		"f":    "mp3",
		"y":    "",
	}).OverWriteOutput().Run()

	if err != nil {
		return fmt.Errorf("ffmpeg recording failed: %w", err)
	}

	return nil
}

// Note: Removed process-based duplicate checking in favor of file-based checking
// This is simpler and more reliable than parsing process lists

// cleanupOldFiles removes old recording files based on retention policy
func (r *Recorder) cleanupOldFiles(streamName, streamDir string) {
	log := r.logger.WithStation(streamName)
	keepDays := r.config.GetStreamKeepDays(streamName)
	
	cutoff := time.Now().AddDate(0, 0, -keepDays)
	
	err := filepath.Walk(streamDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if info.IsDir() {
			return nil
		}
		
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err != nil {
				log.WithFields(logrus.Fields{
					"file": path,
					"error": err,
				}).Warn("Failed to remove old file")
			} else {
				log.WithField("file", path).Debug("Removed old file")
			}
		}
		
		return nil
	})
	
	if err != nil {
		log.Error("Cleanup failed: ", err)
	} else {
		log.Info("Cleanup completed")
	}
}