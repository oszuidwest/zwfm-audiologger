package logger

import (
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

// Logger wraps logrus.Logger with station-specific formatting
type Logger struct {
	*logrus.Logger
}

// New creates a new logger instance
func New(logFile string, debug bool) *Logger {
	log := logrus.New()
	
	// Create log directory if it doesn't exist
	if logFile != "" {
		if err := os.MkdirAll(filepath.Dir(logFile), 0755); err == nil {
			file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err == nil {
				log.SetOutput(file)
			}
		}
	}

	// Set output format
	log.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
	})

	// Set debug level
	if debug {
		log.SetLevel(logrus.DebugLevel)
	} else {
		log.SetLevel(logrus.InfoLevel)
	}

	return &Logger{Logger: log}
}

// WithStation creates a logger with station context
func (l *Logger) WithStation(station string) *logrus.Entry {
	if station == "" {
		station = "GLOBAL"
	}
	return l.WithField("station", station)
}