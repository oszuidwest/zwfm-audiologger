// Package main is the entry point for the audio recorder application
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	_ "time/tzdata" // Embed timezone data for consistent behavior

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/postprocessor"
	"github.com/oszuidwest/zwfm-audiologger/internal/recorder"
	"github.com/oszuidwest/zwfm-audiologger/internal/scheduler"
	"github.com/oszuidwest/zwfm-audiologger/internal/server"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
)

func init() {
	// Go 1.25+ automatically handles container CPU limits for GOMAXPROCS
	// This provides optimal performance in containerized environments
	maxProcs := runtime.GOMAXPROCS(0)
	log.Printf("Starting with GOMAXPROCS=%d (Go %s)", maxProcs, runtime.Version())
}

func main() {
	// Parse command-line flags
	configFile := flag.String("config", "config.json", "Config file path")
	testMode := flag.Bool("test", false, "Test recording (10 seconds)")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Set the timezone from config
	if err := utils.SetTimezone(cfg.Timezone); err != nil {
		log.Printf("Warning: %v", err)
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	// Initialize components
	recorderManager := recorder.New(cfg)
	postProcessor := postprocessor.New(cfg.RecordingsDir)

	// Run test mode if requested
	if *testMode {
		recorderManager.Test(ctx)
		return
	}

	// Start components
	var wg sync.WaitGroup

	// Start HTTP server for trigger endpoints
	wg.Add(1)
	go func() {
		defer wg.Done()
		httpServer := server.New(cfg, recorderManager, postProcessor)
		if err := httpServer.Start(); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start scheduler for ALL stations (always record as failsafe)
	wg.Add(1)
	go func() {
		defer wg.Done()
		sched := scheduler.New(cfg, recorderManager, postProcessor)
		sched.Start(ctx)
	}()

	// Wait for all components to finish
	wg.Wait()
}
