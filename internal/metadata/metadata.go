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
	log := f.logger.WithStation(streamName)
	
	if stream.MetadataURL == "" {
		log.Debug("No metadata URL configured")
		return
	}

	log.Debug(fmt.Sprintf("Fetching metadata from: %s", stream.MetadataURL))
	log.Debug(fmt.Sprintf("Parse metadata: %t, JSON path: %s", stream.ParseMetadata, stream.MetadataJSONPath))

	programName := f.fetchProgramName(stream)
	if programName == "" {
		log.Warn("No program name found, using fallback")
		programName = "Unknown Program"
	}

	// Write metadata to file
	metaFile := filepath.Join(streamDir, timestamp+".meta")
	if err := os.WriteFile(metaFile, []byte(programName), 0644); err != nil {
		log.Errorf("Failed to write metadata file: %v", err)
		return
	}

	log.Infof("Stored metadata - %s - %s", timestamp, programName)
}

// fetchProgramName fetches the program name from the metadata URL
func (f *Fetcher) fetchProgramName(stream config.Stream) string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", stream.MetadataURL, nil)
	if err != nil {
		f.logger.Errorf("Failed to create metadata request: %v", err)
		return ""
	}

	resp, err := f.client.Do(req)
	if err != nil {
		f.logger.Errorf("Failed to fetch metadata: %v", err)
		return ""
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			f.logger.Errorf("Failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		f.logger.Errorf("Metadata request failed with status: %d", resp.StatusCode)
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		f.logger.Errorf("Failed to read metadata response: %v", err)
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
			f.logger.Errorf("JSON path '%s' not found in response", jsonPath)
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