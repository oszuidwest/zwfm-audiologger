// Package metadata handles fetching and parsing metadata from external APIs.
package metadata

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
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
func (f *Fetcher) Fetch(ctx context.Context, url, jsonPath string, parseJSON bool) string {
	if url == "" {
		return ""
	}

	if parseJSON && jsonPath != "" {
		return f.fetchAndParseJSON(ctx, url, jsonPath)
	}
	return f.fetchRaw(ctx, url)
}

// fetchURL retrieves raw content from a URL
func (f *Fetcher) fetchURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", url, err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", url, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body from %s: %w", url, err)
	}

	return body, nil
}

// fetchRaw retrieves raw content from a URL.
func (f *Fetcher) fetchRaw(ctx context.Context, url string) string {
	body, err := f.fetchURL(ctx, url)
	if err != nil {
		slog.Error("failed to fetch metadata", "url", url, "error", err)
		return ""
	}

	return strings.TrimSpace(string(body))
}

// fetchAndParseJSON retrieves and parses JSON from a URL.
func (f *Fetcher) fetchAndParseJSON(ctx context.Context, url, jsonPath string) string {
	body, err := f.fetchURL(ctx, url)
	if err != nil {
		slog.Error("failed to fetch metadata", "url", url, "error", err)
		return ""
	}

	// If no path specified, return the raw body
	if jsonPath == "" {
		return strings.TrimSpace(string(body))
	}

	// Use gjson to extract value at path
	result := gjson.GetBytes(body, jsonPath)
	if !result.Exists() {
		slog.Warn("JSON path not found in metadata", "url", url, "json_path", jsonPath)
		return ""
	}

	return strings.TrimSpace(result.String())
}
