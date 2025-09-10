// Package utils provides common utility functions to reduce code duplication
package utils

import (
	"context"
	"fmt"
	"log/slog"
)

var defaultLogger = slog.Default()

// LogError provides modern structured error logging and returns the error
func LogError(ctx context.Context, action string, err error, attrs ...slog.Attr) error {
	formattedErr := fmt.Errorf("failed to %s: %w", action, err)

	logAttrs := []any{
		slog.String("action", action),
		slog.Any("error", err),
	}
	for _, attr := range attrs {
		logAttrs = append(logAttrs, attr)
	}

	defaultLogger.ErrorContext(ctx, formattedErr.Error(), logAttrs...)
	return formattedErr
}

// LogErrorContinue logs an error but continues execution (doesn't return error)
func LogErrorContinue(ctx context.Context, action string, err error, attrs ...slog.Attr) {
	LogError(ctx, action, err, attrs...)
}
