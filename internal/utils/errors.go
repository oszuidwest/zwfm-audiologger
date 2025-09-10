// Package utils provides common utility functions to reduce code duplication
package utils

import (
	"fmt"
	"log"
)

// LogErrorf logs an error with a formatted message and returns a new error
func LogErrorf(action string, err error) error {
	formattedErr := fmt.Errorf("failed to %s: %w", action, err)
	log.Printf("%v", formattedErr)
	return formattedErr
}

// LogErrorAndContinue logs an error and is used when continuing execution
func LogErrorAndContinue(action string, err error) {
	log.Printf("Failed to %s: %v", action, err)
}
