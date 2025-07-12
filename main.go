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
	configFile := flag.String("config", "streams.json", "Path to configuration file")
	testRecord := flag.Bool("test-record", false, "Run a single test recording and exit")
	flag.Parse()

	cfg, err := config.Load(*configFile)
	if err != nil {
		panic(err)
	}

	log := logger.New(cfg.LogFile, cfg.Debug)
	defer func() {
		if err := log.Close(); err != nil {
			panic(err)
		}
	}()

	log.Info("Starting ZuidWest FM Audio Logger")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Info("Shutdown signal received")
		cancel()
	}()

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

	var wg sync.WaitGroup

	// Unified architecture: Run recorder and HTTP server concurrently
	// Recorder: Cron-scheduled hourly recordings at minute 0
	// Server: Real-time API access to recordings and live segment generation
	wg.Add(1)
	go func() {
		defer wg.Done()
		rec := recorder.New(cfg, log)
		log.Info("Starting continuous recording service")

		if err := rec.StartCron(ctx); err != nil {
			log.Error("recording service failed", "error", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		srv := server.New(cfg, log)
		log.Info("starting HTTP server", "port", cfg.Server.Port)

		if err := srv.Start(ctx, strconv.Itoa(cfg.Server.Port)); err != nil {
			log.Error("HTTP server failed", "error", err)
		}
	}()

	wg.Wait()
	log.Info("ZuidWest FM Audio Logger stopped")
}
