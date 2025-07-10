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

	programName := f.fetchProgramName(stream)
	if programName == "" {
		programName = "Unknown Program"
	}

	// Write metadata to file
	metaFile := filepath.Join(streamDir, timestamp+".meta")
	if err := os.WriteFile(metaFile, []byte(programName), 0644); err != nil {
		log.Error("Failed to write metadata file: ", err)
		return
	}

	log.Info("Stored metadata - ", timestamp, " - ", programName)
}

// fetchProgramName fetches the program name from the metadata URL
func (f *Fetcher) fetchProgramName(stream config.Stream) string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", stream.MetadataURL, nil)
	if err != nil {
		f.logger.Error("Failed to create metadata request: ", err)
		return ""
	}

	resp, err := f.client.Do(req)
	if err != nil {
		f.logger.Error("Failed to fetch metadata: ", err)
		return ""
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			f.logger.Error("Failed to close response body: ", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		f.logger.Error("Metadata request failed with status: ", resp.StatusCode)
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		f.logger.Error("Failed to read metadata response: ", err)
		return ""
	}

	bodyStr := string(body)
	
	// Parse JSON if parse_metadata is enabled and path is provided
	if stream.ParseMetadata == 1 && stream.MetadataJSONPath != "" {
		result := gjson.Get(bodyStr, stream.MetadataJSONPath)
		if result.Exists() {
			return result.String()
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