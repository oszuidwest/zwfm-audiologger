package metadata

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/logger"
	"github.com/oszuidwest/zwfm-audiologger/internal/version"
	"github.com/tidwall/gjson"
)

type Fetcher struct {
	logger *logger.Logger
	client *http.Client
}

// New returns a new metadata Fetcher with a 15-second HTTP timeout.
func New(log *logger.Logger) *Fetcher {
	return &Fetcher{
		logger: log,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// FetchMetadata retrieves program metadata for a recording and stores it as a .meta file.
func (f *Fetcher) FetchMetadata(stationName string, station config.Station, stationDir, timestamp string) {
	if station.MetadataURL == "" {
		f.logger.Debug("no metadata URL configured", "station", stationName)
		return
	}

	f.logger.Debug("fetching metadata", "station", stationName, "url", station.MetadataURL, "parse", station.ParseMetadata, "path", station.MetadataJSONPath)

	programName := f.fetchProgramName(station)
	if programName == "" {
		f.logger.Warn("no program name found, using fallback", "station", stationName)
		programName = "Unknown Program"
	}

	metaFile := filepath.Join(stationDir, timestamp+".meta")
	if err := os.WriteFile(metaFile, []byte(programName), 0644); err != nil {
		f.logger.Error("failed to write metadata file", "station", stationName, "file", metaFile, "error", err)
		return
	}

	f.logger.Info("stored metadata", "station", stationName, "timestamp", timestamp, "program", programName)
}

// fetchProgramName retrieves the program name from a station's metadata API.
func (f *Fetcher) fetchProgramName(station config.Station) string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", station.MetadataURL, nil)
	if err != nil {
		f.logger.Error("failed to create metadata request", "error", err)
		return ""
	}
	req.Header.Set("User-Agent", version.UserAgent())

	resp, err := f.client.Do(req)
	if err != nil {
		f.logger.Error("failed to fetch metadata", "error", err)
		return ""
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			f.logger.Error("failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		f.logger.Error("metadata request failed", "status_code", resp.StatusCode)
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		f.logger.Error("failed to read metadata response", "error", err)
		return ""
	}

	bodyStr := string(body)

	// Parse JSON response using gjson path syntax if configured
	// Example: "program.title" extracts {"program": {"title": "Morning Show"}} -> "Morning Show"
	if station.ParseMetadata && station.MetadataJSONPath != "" {
		// Remove leading dot from path for gjson compatibility
		jsonPath := strings.TrimPrefix(station.MetadataJSONPath, ".")

		f.logger.Debug(fmt.Sprintf("Parsing metadata with path '%s' from JSON: %s", jsonPath, bodyStr))

		// Use gjson to extract value at specified path
		result := gjson.Get(bodyStr, jsonPath)
		if result.Exists() {
			f.logger.Debug(fmt.Sprintf("Parsed metadata result: %s", result.String()))
			return result.String()
		} else {
			f.logger.Error("JSON path not found in response", "path", jsonPath)
		}
	}

	return strings.TrimSpace(bodyStr)
}

// GetMetadata reads a metadata file for the given timestamp.
func (f *Fetcher) GetMetadata(stationDir, timestamp string) (string, error) {
	metaFile := filepath.Join(stationDir, timestamp+".meta")

	data, err := os.ReadFile(metaFile)
	if err != nil {
		return "", fmt.Errorf("failed to read metadata file: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}
