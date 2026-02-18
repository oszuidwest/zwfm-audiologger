// Package validator provides audio recording validation functionality.
package validator

import (
	"encoding/json"
	"os"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
)

// ValidationResult holds the results of a recording validation.
type ValidationResult struct {
	Station        string    `json:"station"`
	Timestamp      string    `json:"timestamp"`
	ValidatedAt    time.Time `json:"validated_at"`
	DurationSecs   float64   `json:"duration_secs"`
	SilencePercent float64   `json:"silence_percent"`
	LoopPercent    float64   `json:"loop_percent"`
	Valid          bool      `json:"valid"`
	Issues         []string  `json:"issues,omitempty"`
}

// Save writes the validation result to a JSON file.
func (r *ValidationResult) Save(path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, constants.FilePermissions)
}
