// Package main is the entry point for the audio recorder application.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	_ "time/tzdata" // Ensures timezone functionality across all platforms

	"github.com/oszuidwest/zwfm-audiologger/internal/alerting"
	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/recorder"
	"github.com/oszuidwest/zwfm-audiologger/internal/scheduler"
	"github.com/oszuidwest/zwfm-audiologger/internal/server"
	"github.com/oszuidwest/zwfm-audiologger/internal/timeutil"
)

// Build information variables set via ldflags during build.
var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	// Initialize structured logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Parse command-line flags
	configFile := flag.String("config", "config.json", "Config file path")
	testMode := flag.Bool("test", false, "Test recording (10 seconds)")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	// Show version if requested
	if *showVersion {
		fmt.Printf("ZuidWest FM Audio Logger\n")
		fmt.Printf("Version:    %s\n", version)
		fmt.Printf("Build Time: %s\n", buildTime)
		fmt.Printf("Git Commit: %s\n", gitCommit)
		fmt.Printf("Go Version: %s\n", runtime.Version())
		fmt.Printf("Platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
		return
	}

	// Load configuration
	cfg, err := config.Load(*configFile)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Set the timezone from config
	if err := timeutil.SetTimezone(cfg.Timezone); err != nil {
		slog.Warn("failed to set timezone", "error", err)
	}

	// Create context for graceful shutdown with signal handling
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Initialize alerter from config
	alerterConfig := &alerting.Config{
		WebhookURL: cfg.Alerting.WebhookURL,
	}
	// Only configure email alerter if explicitly enabled
	if cfg.Alerting.Email.Enabled {
		alerterConfig.Email = &cfg.Alerting.Email
	}
	alerter := alerting.NewManager(alerterConfig)

	// Initialize components
	recorderManager := recorder.New(cfg, alerter)

	// Run test mode if requested
	if *testMode {
		recorderManager.Test(ctx)
		return
	}

	// Start components concurrently using goroutines
	var wg sync.WaitGroup

	// Start HTTP server for trigger endpoints
	httpServer := server.New(cfg, recorderManager)
	wg.Go(func() {
		if err := httpServer.Start(ctx); err != nil {
			slog.Error("HTTP server error", "error", err)
		}
	})

	// Start scheduler for ALL stations (always record as failsafe)
	wg.Go(func() {
		sched := scheduler.New(cfg, recorderManager, alerter)
		if err := sched.Start(ctx); err != nil {
			slog.Error("Scheduler error", "error", err)
		}
	})

	// Wait for all components to finish
	wg.Wait()
}
