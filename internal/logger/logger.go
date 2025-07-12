package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

type Logger struct {
	slog *slog.Logger
	file *os.File
}

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

func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

func (l *Logger) Info(msg string, args ...any) {
	l.slog.Info(msg, args...)
}

func (l *Logger) Warn(msg string, args ...any) {
	l.slog.Warn(msg, args...)
}

func (l *Logger) Error(msg string, args ...any) {
	l.slog.Error(msg, args...)
}

func (l *Logger) Debug(msg string, args ...any) {
	l.slog.Debug(msg, args...)
}

func (l *Logger) Fatal(msg string, args ...any) {
	l.slog.Error(msg, args...)
	os.Exit(1)
}

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
