package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/logger"
	"github.com/oszuidwest/zwfm-audiologger/internal/recorder"
	"github.com/oszuidwest/zwfm-audiologger/internal/server"
)

func main() {
	// Define command line flags
	configFile := flag.String("config", "streams.json", "Path to configuration file")
	testRecord := flag.Bool("test-record", false, "Run a single test recording and exit")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configFile)
	if err != nil {
		panic(err)
	}

	// Initialize logger
	log := logger.New(cfg.LogFile, cfg.Debug)
	defer func() {
		if err := log.Close(); err != nil {
			// Can't use logger here since we're closing it
			panic(err)
		}
	}()

	log.Info("Starting ZuidWest FM Audio Logger")

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Info("Shutdown signal received")
		cancel()
	}()

	// If test recording flag is set, run a single recording test
	if *testRecord {
		rec := recorder.New(cfg, log)
		log.Info("Running test recording")

		if err := rec.RecordAll(ctx); err != nil {
			log.Error("test recording failed", "error", err)
			os.Exit(1)
		}

		log.Info("Test recording completed")
		return
	}

	// Start both recorder and HTTP server
	var wg sync.WaitGroup

	// Start recording service
	wg.Add(1)
	go func() {
		defer wg.Done()
		rec := recorder.New(cfg, log)
		log.Info("Starting continuous recording service")

		if err := rec.StartCron(ctx); err != nil {
			log.Error("recording service failed", "error", err)
		}
	}()

	// Start HTTP server
	wg.Add(1)
	go func() {
		defer wg.Done()
		srv := server.New(cfg, log)
		log.Info("starting HTTP server", "port", cfg.Server.Port)

		if err := srv.Start(ctx, strconv.Itoa(cfg.Server.Port)); err != nil {
			log.Error("HTTP server failed", "error", err)
		}
	}()

	// Wait for both services to complete
	wg.Wait()
	log.Info("ZuidWest FM Audio Logger stopped")
}
