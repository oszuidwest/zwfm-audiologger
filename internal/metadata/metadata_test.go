package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchRawTrimsWhitespace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("  Artist - Title \n"))
	}))
	t.Cleanup(server.Close)

	got := New().Fetch(context.Background(), server.URL, "", false)
	if got != "Artist - Title" {
		t.Errorf("Fetch raw = %q, want %q", got, "Artist - Title")
	}
}

func TestFetchParsesJSONPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"current":{"title":" Artist - Title "}}}`))
	}))
	t.Cleanup(server.Close)

	got := New().Fetch(context.Background(), server.URL, "data.current.title", true)
	if got != "Artist - Title" {
		t.Errorf("Fetch JSON path = %q, want %q", got, "Artist - Title")
	}
}

func TestFetchRespectsMidFlightCancelledContext(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())

	resultCh := make(chan string, 1)
	go func() {
		resultCh <- New().Fetch(ctx, server.URL, "", false)
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("metadata request did not reach test server")
	}
	cancel()

	select {
	case got := <-resultCh:
		if got != "" {
			t.Errorf("Fetch with cancelled context = %q, want empty string", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Fetch did not return after context cancellation")
	}
}

func TestFetchReturnsEmptyForHTTPErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
		_, _ = w.Write([]byte(strings.Repeat("x", maxMetadataResponseBytes+1)))
	}))
	t.Cleanup(server.Close)

	got := New().Fetch(context.Background(), server.URL, "", false)
	if got != "" {
		t.Errorf("Fetch HTTP error = %q, want empty string", got)
	}
}

func TestFetchReturnsEmptyForInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":`))
	}))
	t.Cleanup(server.Close)

	got := New().Fetch(context.Background(), server.URL, "data.current.title", true)
	if got != "" {
		t.Errorf("Fetch invalid JSON = %q, want empty string", got)
	}
}

func TestFetchReturnsEmptyForMissingJSONPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"other":"value"}}`))
	}))
	t.Cleanup(server.Close)

	got := New().Fetch(context.Background(), server.URL, "data.current.title", true)
	if got != "" {
		t.Errorf("Fetch missing JSON path = %q, want empty string", got)
	}
}

func TestFetchReturnsEmptyForOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxMetadataResponseBytes+1)))
	}))
	t.Cleanup(server.Close)

	got := New().Fetch(context.Background(), server.URL, "", false)
	if got != "" {
		t.Errorf("Fetch oversized response returned %d bytes, want empty string", len(got))
	}
}

func TestExtractJSONPathReportsTypeMismatch(t *testing.T) {
	_, err := extractJSONPath([]byte(`{"data":{"current":{"title":42}}}`), "data.current.title")
	if err == nil {
		t.Fatal("extractJSONPath returned nil error for non-string final value")
	}
	if !strings.Contains(err.Error(), "has float64 value, want string") {
		t.Fatalf("extractJSONPath error = %q, want type mismatch", err)
	}
}
