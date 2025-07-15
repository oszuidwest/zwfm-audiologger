// Package logger provides structured logging functionality using Go's standard
// slog library with support for file and console output.
package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// Logger wraps slog.Logger with file handling and provides structured logging
// methods for consistent log output formatting.
type Logger struct {
	slog *slog.Logger
	file *os.File
}

// New returns a new Logger that writes to logFile and stdout.
// If debug is true, the logger includes debug-level messages.
func New(logFile string, debug bool) *Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	var writer io.Writer = os.Stdout
	var file *os.File

	if logFile != "" {
		if err := os.MkdirAll(filepath.Dir(logFile), 0755); err == nil {
			if f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
				file = f
				writer = io.MultiWriter(os.Stdout, f)
			}
		}
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	handler := slog.NewTextHandler(writer, opts)
	logger := slog.New(handler)

	return &Logger{
		slog: logger,
		file: file,
	}
}

// Close closes the log file if one was opened.
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Info logs a message at INFO level with optional key-value pairs.
func (l *Logger) Info(msg string, args ...any) {
	l.slog.Info(msg, args...)
}

// Warn logs a message at WARN level with optional key-value pairs.
func (l *Logger) Warn(msg string, args ...any) {
	l.slog.Warn(msg, args...)
}

// Error logs a message at ERROR level with optional key-value pairs.
func (l *Logger) Error(msg string, args ...any) {
	l.slog.Error(msg, args...)
}

// Debug logs a message at DEBUG level with optional key-value pairs.
func (l *Logger) Debug(msg string, args ...any) {
	l.slog.Debug(msg, args...)
}

// HTTPRequest logs HTTP request details with appropriate log levels based on status code.
func (l *Logger) HTTPRequest(method, path string, statusCode int, duration time.Duration, requestID string) {
	level := slog.LevelInfo
	if statusCode >= 400 {
		level = slog.LevelWarn
	}
	if statusCode >= 500 {
		level = slog.LevelError
	}

	l.slog.Log(context.Background(), level, "http request",
		"method", method,
		"path", path,
		"status", statusCode,
		"duration_ms", duration.Milliseconds(),
		"request_id", requestID,
	)
}
