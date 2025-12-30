// Package server provides HTTP endpoints for controlling recordings.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/recorder"
)

// Server is an HTTP server that provides recording control and status endpoints.
type Server struct {
	config   *config.Config
	recorder *recorder.Manager
	mux      *http.ServeMux
}

// New creates a new HTTP server.
func New(cfg *config.Config, rec *recorder.Manager) *Server {
	s := &Server{
		config:   cfg,
		recorder: rec,
		mux:      http.NewServeMux(),
	}

	// Setup routes
	s.setupRoutes()

	return s
}

// setupRoutes configures the HTTP routes.
func (s *Server) setupRoutes() {
	s.mux.HandleFunc("GET /status", s.handleStatus)
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// Serve recordings using stdlib http.FileServer, filtering out temp files
	fileServer := http.FileServer(http.Dir(s.config.RecordingsDir))
	s.mux.Handle("GET /recordings/", http.StripPrefix("/recordings", s.filterTempFiles(fileServer)))
}

// filterTempFiles wraps a handler to reject requests for temporary .mkv files.
func (s *Server) filterTempFiles(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block access to .mkv temp files (recordings in progress)
		if strings.HasSuffix(strings.ToLower(filepath.Clean(r.URL.Path)), ".mkv") {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Start begins listening for HTTP requests.
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.config.Port)

	slog.Info("HTTP server listening", "port", s.config.Port)
	slog.Info("Endpoints:")
	slog.Info("  - GET /recordings/* (browse recordings)")
	slog.Info("  - GET /status (system status)")
	slog.Info("  - GET /health (health check)")

	server := &http.Server{
		Addr:              addr,
		Handler:           s.mux,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      10 * time.Minute, // Large timeout for streaming audio files
		IdleTimeout:       60 * time.Second,
	}

	// Handle graceful shutdown
	go func() {
		<-ctx.Done()
		slog.Info("Shutting down HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("http server shutdown error", "error", err)
		}
	}()

	return server.ListenAndServe()
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}
