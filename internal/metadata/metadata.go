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
	"github.com/tidwall/gjson"
)

// Fetcher handles metadata fetching from APIs
type Fetcher struct {
	logger *logger.Logger
	client *http.Client
}

// New creates a new metadata fetcher
func New(log *logger.Logger) *Fetcher {
	return &Fetcher{
		logger: log,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Fetch fetches metadata for a stream and saves it to a .meta file
func (f *Fetcher) Fetch(streamName string, stream config.Stream, streamDir, timestamp string) {
	if stream.MetadataURL == "" {
		f.logger.Debug("no metadata URL configured", "station", streamName)
		return
	}

	f.logger.Debug("fetching metadata", "station", streamName, "url", stream.MetadataURL, "parse", stream.ParseMetadata, "path", stream.MetadataJSONPath)

	programName := f.fetchProgramName(stream)
	if programName == "" {
		f.logger.Warn("no program name found, using fallback", "station", streamName)
		programName = "Unknown Program"
	}

	// Write metadata to file
	metaFile := filepath.Join(streamDir, timestamp+".meta")
	if err := os.WriteFile(metaFile, []byte(programName), 0644); err != nil {
		f.logger.Error("failed to write metadata file", "station", streamName, "file", metaFile, "error", err)
		return
	}

	f.logger.Info("stored metadata", "station", streamName, "timestamp", timestamp, "program", programName)
}

// fetchProgramName fetches the program name from the metadata URL
func (f *Fetcher) fetchProgramName(stream config.Stream) string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", stream.MetadataURL, nil)
	if err != nil {
		f.logger.Error("failed to create metadata request", "error", err)
		return ""
	}

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

	// Parse JSON if parse_metadata is enabled and path is provided
	if stream.ParseMetadata && stream.MetadataJSONPath != "" {
		// Clean the JSON path (remove leading dot if present)
		jsonPath := strings.TrimPrefix(stream.MetadataJSONPath, ".")

		f.logger.Debug(fmt.Sprintf("Parsing metadata with path '%s' from JSON: %s", jsonPath, bodyStr))

		result := gjson.Get(bodyStr, jsonPath)
		if result.Exists() {
			f.logger.Debug(fmt.Sprintf("Parsed metadata result: %s", result.String()))
			return result.String()
		} else {
			f.logger.Error("JSON path not found in response", "path", jsonPath)
		}
	}

	// Return raw response if parse_metadata is disabled or parsing failed
	return strings.TrimSpace(bodyStr)
}

// GetMetadata retrieves metadata for a specific recording
func (f *Fetcher) GetMetadata(streamDir, timestamp string) (string, error) {
	metaFile := filepath.Join(streamDir, timestamp+".meta")

	data, err := os.ReadFile(metaFile)
	if err != nil {
		return "", fmt.Errorf("failed to read metadata file: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}
