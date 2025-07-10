package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Logger wraps slog for simple, clean logging
type Logger struct {
	slog *slog.Logger
	file *os.File
}

// New creates a new logger using Go's standard slog
func New(logFile string, debug bool) *Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	var writer io.Writer = os.Stdout
	var file *os.File

	// Set up file logging if specified
	if logFile != "" {
		if err := os.MkdirAll(filepath.Dir(logFile), 0755); err == nil {
			if f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
				file = f
				writer = io.MultiWriter(os.Stdout, f)
			}
		}
	}

	// Create text handler for clean, readable output
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

// Close closes the log file if open
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Debug logs a debug message
func (l *Logger) Debug(msg string) {
	l.slog.Debug(msg)
}

// Info logs an info message
func (l *Logger) Info(msg string) {
	l.slog.Info(msg)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string) {
	l.slog.Warn(msg)
}

// Error logs an error message
func (l *Logger) Error(msg string) {
	l.slog.Error(msg)
}

// Fatal logs a fatal error and exits
func (l *Logger) Fatal(msg string) {
	l.slog.Error(msg)
	os.Exit(1)
}

// StationLogger provides station-specific logging
type StationLogger struct {
	slog *slog.Logger
}

// WithStation creates a logger with station context
func (l *Logger) WithStation(station string) *StationLogger {
	return &StationLogger{
		slog: l.slog.With("station", station),
	}
}

// Debug logs a debug message with station context
func (s *StationLogger) Debug(msg string) {
	s.slog.Debug(msg)
}

// Info logs an info message with station context
func (s *StationLogger) Info(msg string) {
	s.slog.Info(msg)
}

// Warn logs a warning message with station context
func (s *StationLogger) Warn(msg string) {
	s.slog.Warn(msg)
}

// Error logs an error message with station context
func (s *StationLogger) Error(msg string) {
	s.slog.Error(msg)
}

// Convenience methods for formatted logging
func (l *Logger) Infof(format string, args ...interface{}) {
	l.slog.Info(fmt.Sprintf(format, args...))
}

func (l *Logger) Warnf(format string, args ...interface{}) {
	l.slog.Warn(fmt.Sprintf(format, args...))
}

func (l *Logger) Errorf(format string, args ...interface{}) {
	l.slog.Error(fmt.Sprintf(format, args...))
}

func (l *Logger) Debugf(format string, args ...interface{}) {
	l.slog.Debug(fmt.Sprintf(format, args...))
}

func (s *StationLogger) Infof(format string, args ...interface{}) {
	s.slog.Info(fmt.Sprintf(format, args...))
}

func (s *StationLogger) Warnf(format string, args ...interface{}) {
	s.slog.Warn(fmt.Sprintf(format, args...))
}

func (s *StationLogger) Errorf(format string, args ...interface{}) {
	s.slog.Error(fmt.Sprintf(format, args...))
}

func (s *StationLogger) Debugf(format string, args ...interface{}) {
	s.slog.Debug(fmt.Sprintf(format, args...))
}