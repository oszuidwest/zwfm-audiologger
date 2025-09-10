// Package metadata handles fetching and parsing metadata from external APIs
package metadata

import (
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
	"github.com/tidwall/gjson"
)

// Fetcher handles metadata retrieval from external sources
type Fetcher struct {
	client *http.Client
}

// New creates a new metadata fetcher
func New() *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Fetch retrieves metadata from the given URL and optionally parses JSON
func (f *Fetcher) Fetch(url string, jsonPath string, parseJSON bool) string {
	if url == "" {
		return ""
	}

	if parseJSON && jsonPath != "" {
		return f.fetchAndParseJSON(url, jsonPath)
	}
	return f.fetchRaw(url)
}

// fetchRaw retrieves raw content from a URL
func (f *Fetcher) fetchRaw(url string) string {
	resp, err := f.client.Get(url)
	if err != nil {
		utils.LogErrorAndContinue("fetch metadata", err)
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		utils.LogErrorAndContinue("read metadata response", err)
		return ""
	}

	return strings.TrimSpace(string(body))
}

// fetchAndParseJSON retrieves and parses JSON from a URL using gjson
func (f *Fetcher) fetchAndParseJSON(url, jsonPath string) string {
	resp, err := f.client.Get(url)
	if err != nil {
		utils.LogErrorAndContinue("fetch metadata", err)
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		utils.LogErrorAndContinue("read metadata response", err)
		return ""
	}

	// If no JSON path specified, return raw response
	if jsonPath == "" {
		return strings.TrimSpace(string(body))
	}

	// Use gjson to extract value at path
	result := gjson.GetBytes(body, jsonPath)
	if !result.Exists() {
		log.Printf("JSON path '%s' not found in metadata", jsonPath)
		return ""
	}

	// Return the string representation
	return result.String()
}
