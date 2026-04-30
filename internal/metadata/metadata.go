// Package metadata handles fetching and parsing metadata from external APIs.
package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const maxMetadataResponseBytes = 1 << 20

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

// fetchURL retrieves raw content from a URL.
func (f *Fetcher) fetchURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		drainBody(resp.Body)
		return nil, fmt.Errorf("metadata url returned http status %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxMetadataResponseBytes {
		drainBody(resp.Body)
		return nil, fmt.Errorf("metadata response exceeds %d bytes", maxMetadataResponseBytes)
	}

	return body, nil
}

func drainBody(body io.Reader) {
	_, _ = io.Copy(io.Discard, body)
}

// fetchRaw retrieves raw content from a URL.
func (f *Fetcher) fetchRaw(ctx context.Context, url string) string {
	body, err := f.fetchURL(ctx, url)
	if err != nil {
		logFetchError(ctx, err)
		return ""
	}

	return strings.TrimSpace(string(body))
}

// fetchAndParseJSON retrieves and parses JSON from a URL.
func (f *Fetcher) fetchAndParseJSON(ctx context.Context, url, jsonPath string) string {
	body, err := f.fetchURL(ctx, url)
	if err != nil {
		logFetchError(ctx, err)
		return ""
	}

	// Parse JSON and extract value at path.
	value, err := extractJSONPath(body, jsonPath)
	if err != nil {
		slog.Warn("failed to parse metadata JSON", "json_path", jsonPath, "error", err)
	}

	return value
}

func logFetchError(ctx context.Context, err error) {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		slog.Info("metadata fetch stopped", "error", err)
		return
	}
	slog.Error("failed to fetch metadata", "error", err)
}

// extractJSONPath extracts a value from JSON using simple dot notation.
func extractJSONPath(data []byte, path string) (string, error) {
	if path == "" {
		return strings.TrimSpace(string(data)), nil
	}

	var jsonData map[string]any
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return "", fmt.Errorf("parse metadata json: %w", err)
	}

	parts := strings.Split(path, ".")
	current := jsonData

	for i, part := range parts {
		isLastPart := i == len(parts)-1
		value, ok := current[part]
		if !ok {
			return "", fmt.Errorf("json path %q not found", path)
		}

		if isLastPart {
			text, ok := value.(string)
			if !ok {
				return "", fmt.Errorf("json path %q has %T value, want string", path, value)
			}
			return strings.TrimSpace(text), nil
		}

		next, ok := value.(map[string]any)
		if !ok {
			return "", fmt.Errorf("json path %q has %T at %q, want object", path, value, part)
		}
		current = next
	}

	return "", fmt.Errorf("json path %q not found", path)
}
