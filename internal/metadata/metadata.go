// Package metadata handles fetching and parsing metadata from external APIs.
package metadata

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Fetcher handles metadata retrieval from external sources.
type Fetcher struct {
	client *http.Client
}

// New creates a new metadata fetcher.
func New() *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Fetch retrieves metadata from the given URL and optionally parses JSON.
func (f *Fetcher) Fetch(url, jsonPath string, parseJSON bool) string {
	if url == "" {
		return ""
	}

	if parseJSON && jsonPath != "" {
		return f.fetchAndParseJSON(url, jsonPath)
	}
	return f.fetchRaw(url)
}

// fetchURL retrieves raw content from a URL
func (f *Fetcher) fetchURL(url string) ([]byte, error) {
	resp, err := f.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// fetchRaw retrieves raw content from a URL.
func (f *Fetcher) fetchRaw(url string) string {
	body, err := f.fetchURL(url)
	if err != nil {
		slog.Error("failed to fetch metadata", "error", err)
		return ""
	}

	return strings.TrimSpace(string(body))
}

// fetchAndParseJSON retrieves and parses JSON from a URL.
func (f *Fetcher) fetchAndParseJSON(url, jsonPath string) string {
	body, err := f.fetchURL(url)
	if err != nil {
		slog.Error("failed to fetch metadata", "error", err)
		return ""
	}

	// Parse JSON and extract value at path.
	value := extractJSONPath(body, jsonPath)
	if value == "" {
		slog.Warn("JSON path not found in metadata", "json_path", jsonPath)
	}

	return value
}

// extractJSONPath extracts a value from JSON using simple dot notation.
func extractJSONPath(data []byte, path string) string {
	if path == "" {
		return strings.TrimSpace(string(data))
	}

	// Parse as generic map for simple dot notation.
	var jsonData map[string]interface{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return ""
	}

	parts := strings.Split(path, ".")
	current := jsonData

	// Navigate through the path.
	for i, part := range parts {
		if i == len(parts)-1 {
			// Last part - extract the value.
			if value, ok := current[part].(string); ok {
				return strings.TrimSpace(value)
			}
			return ""
		}
		// Intermediate part - go deeper.
		if next, ok := current[part].(map[string]interface{}); ok {
			current = next
		} else {
			return ""
		}
	}

	return ""
}
