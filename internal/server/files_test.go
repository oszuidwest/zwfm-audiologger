package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
)

func newRecordingsTestServer(t *testing.T) *Server {
	t.Helper()

	baseDir := t.TempDir()
	recordingsDir := filepath.Join(baseDir, "recordings")
	if err := os.MkdirAll(filepath.Join(recordingsDir, "station"), 0o750); err != nil {
		t.Fatalf("create recordings dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(recordingsDir, "station", "2026-10-22-10.meta"), []byte("0123456789"), 0o600); err != nil {
		t.Fatalf("write recording: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	return &Server{config: &config.Config{RecordingsDir: recordingsDir}}
}

func serveRecordingsPath(s *Server, urlPath, byteRange string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/recordings/"+urlPath, http.NoBody)
	req.SetPathValue("path", urlPath)
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}
	rec := httptest.NewRecorder()
	s.handleRecordings(rec, req)
	return rec
}

func TestHandleRecordingsServesFile(t *testing.T) {
	s := newRecordingsTestServer(t)

	rec := serveRecordingsPath(s, "station/2026-10-22-10.meta", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "0123456789" {
		t.Fatalf("body = %q, want %q", got, "0123456789")
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); got != `inline; filename="2026-10-22-10.meta"` {
		t.Fatalf("Content-Disposition = %q", got)
	}

	rec = serveRecordingsPath(s, "station/2026-10-22-10.meta", "bytes=2-4")
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("range status = %d, want %d", rec.Code, http.StatusPartialContent)
	}
	if got := rec.Body.String(); got != "234" {
		t.Fatalf("range body = %q, want %q", got, "234")
	}
	if got := rec.Header().Get("Content-Range"); got != "bytes 2-4/10" {
		t.Fatalf("Content-Range = %q", got)
	}
}

func TestHandleRecordingsStaysInsideRecordingsDir(t *testing.T) {
	s := newRecordingsTestServer(t)

	for _, urlPath := range []string{"../secret.txt", "station/../../secret.txt"} {
		rec := serveRecordingsPath(s, urlPath, "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want %d", urlPath, rec.Code, http.StatusNotFound)
		}
		if strings.Contains(rec.Body.String(), "secret") {
			t.Errorf("%s: response leaked file outside recordings dir", urlPath)
		}
	}
}
