// Package server provides HTTP endpoints for controlling recordings.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
	"github.com/oszuidwest/zwfm-audiologger/internal/postprocessor"
	"github.com/oszuidwest/zwfm-audiologger/internal/recorder"
)

// Server handles HTTP requests for recording control.
type Server struct {
	config        *config.Config
	recorder      *recorder.Manager
	postProcessor *postprocessor.Manager
	mux           *http.ServeMux
	accessLogger  *slog.Logger
}

// New creates a new HTTP server.
func New(cfg *config.Config, rec *recorder.Manager, pp *postprocessor.Manager) *Server {
	// Create access log file
	accessLogFile, err := os.OpenFile(constants.DefaultAccessLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, constants.LogFilePermissions)
	if err != nil {
		// Fallback to stdout if can't create log file
		slog.Warn("Cannot create access.log, falling back to stdout", "error", err)
		accessLogFile = os.Stdout
	}

	s := &Server{
		config:        cfg,
		recorder:      rec,
		postProcessor: pp,
		mux:           http.NewServeMux(),
		accessLogger:  slog.New(slog.NewJSONHandler(accessLogFile, nil)),
	}

	// Setup routes
	s.setupRoutes()

	return s
}

// setupRoutes configures the HTTP routes.
func (s *Server) setupRoutes() {
	// Public endpoints
	s.mux.HandleFunc("GET /status", s.handleStatus)
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /recordings/{path...}", s.handleRecordings)

	// Protected endpoints with authentication
	s.mux.HandleFunc("POST /program/start/{station}", s.authenticate(s.handleProgramStart))
	s.mux.HandleFunc("POST /program/stop/{station}", s.authenticate(s.handleProgramStop))
}

// Start begins listening for HTTP requests.
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.config.Port)

	slog.Info("HTTP server listening", "port", s.config.Port)
	slog.Info("Endpoints:")
	slog.Info("  - POST /program/start/:station (requires auth)")
	slog.Info("  - POST /program/stop/:station (requires auth)")
	slog.Info("  - GET /recordings/* (browse recordings)")
	slog.Info("  - GET /status (system status)")
	slog.Info("  - GET /health (health check)")

	// Create HTTP server with logging middleware
	server := &http.Server{
		Addr:         addr,
		Handler:      s.loggingMiddleware(s.mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
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

// loggingMiddleware logs HTTP requests.
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap ResponseWriter to capture status code
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(lrw, r)

		s.accessLogger.Info("HTTP request", "method", r.Method, "path", r.URL.Path, "status", lrw.statusCode, "duration", time.Since(start))
	})
}

// loggingResponseWriter wraps http.ResponseWriter to capture status code for logging purposes.
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code and calls the underlying ResponseWriter's WriteHeader.
func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}
