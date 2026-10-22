// Package server provides HTTP endpoints for controlling recordings.
package server

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
)

// Server handles HTTP requests for recording control.
type Server struct {
	config        *config.Config
	mux           *http.ServeMux
	accessLogger  *slog.Logger
	accessLogFile *os.File // nil when falling back to stdout.
}

// New creates a new HTTP server.
func New(cfg *config.Config) *Server {
	s := &Server{
		config: cfg,
		mux:    http.NewServeMux(),
	}

	// Open access log file; fall back to stdout on failure.
	f, err := os.OpenFile(
		constants.DefaultAccessLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, constants.LogFilePermissions,
	)
	if err != nil {
		slog.Warn("Cannot create access.log, falling back to stdout", "error", err)
		s.accessLogger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	} else {
		s.accessLogFile = f
		s.accessLogger = slog.New(slog.NewJSONHandler(f, nil))
	}

	// Setup routes
	s.setupRoutes()

	return s
}

// setupRoutes configures the HTTP routes.
func (s *Server) setupRoutes() {
	s.mux.HandleFunc("GET /status", s.handleStatus)
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /recordings/{path...}", s.handleRecordings)
}

// Start begins listening for HTTP requests.
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.config.Port)

	slog.Info("HTTP server listening", "port", s.config.Port)
	slog.Info("Endpoints:")
	slog.Info("  - GET /recordings/* (browse recordings)")
	slog.Info("  - GET /status (system status)")
	slog.Info("  - GET /health (health check)")

	// Create HTTP server with logging middleware
	server := &http.Server{
		Addr:              addr,
		Handler:           s.loggingMiddleware(s.mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Run ListenAndServe in a goroutine so we can select on context cancellation.
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	// Wait for context cancellation or server error.
	select {
	case err := <-errCh:
		s.closeAccessLogFile()
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
	}

	// Gracefully shut down with a timeout.
	slog.Info("Shutting down HTTP server")
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		// Handlers may still be writing to the access log; the OS closes it on exit.
		return fmt.Errorf("shut down HTTP server: %w", err)
	}
	s.closeAccessLogFile()

	if err := <-errCh; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP: %w", err)
	}
	return nil
}

// closeAccessLogFile closes the access log file, clears the field, and is safe to call repeatedly.
func (s *Server) closeAccessLogFile() {
	if s.accessLogFile == nil {
		return
	}
	path := s.accessLogFile.Name()
	if err := s.accessLogFile.Close(); err != nil {
		slog.Error("failed to close access log file", "path", path, "error", err)
	}
	s.accessLogFile = nil
}

// loggingMiddleware logs HTTP requests.
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		lrw := &loggingResponseWriter{ResponseWriter: w}

		next.ServeHTTP(lrw, r)

		s.accessLogger.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", cmp.Or(lrw.statusCode, http.StatusOK),
			"duration", time.Since(start),
		)
	})
}

// loggingResponseWriter wraps http.ResponseWriter to capture status code for logging purposes.
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int // 0 until the final status is sent; log it as 200 then.
}

func (lrw *loggingResponseWriter) Unwrap() http.ResponseWriter {
	return lrw.ResponseWriter
}

// Write pins the implicit 200 that net/http sends when a body precedes WriteHeader.
func (lrw *loggingResponseWriter) Write(data []byte) (int, error) {
	lrw.statusCode = cmp.Or(lrw.statusCode, http.StatusOK)
	return lrw.ResponseWriter.Write(data)
}

// WriteHeader records the first final status; 1xx (except 101) leave the header open, as in net/http.
func (lrw *loggingResponseWriter) WriteHeader(code int) {
	if lrw.statusCode == 0 && (code < 100 || code >= 200 || code == http.StatusSwitchingProtocols) {
		lrw.statusCode = code
	}
	lrw.ResponseWriter.WriteHeader(code)
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		slog.Error("failed to encode JSON response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}
