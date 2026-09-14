package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
)

func TestStartClosesAccessLogFileWhenListenFails(t *testing.T) {
	accessLogFile, err := os.CreateTemp(t.TempDir(), "access-*.log")
	if err != nil {
		t.Fatalf("create access log: %v", err)
	}
	// The server should close this first; cleanup is only a fallback.
	t.Cleanup(func() { _ = accessLogFile.Close() })

	s := &Server{
		config:        &config.Config{Port: -1},
		mux:           http.NewServeMux(),
		accessLogger:  slog.New(slog.NewJSONHandler(accessLogFile, nil)),
		accessLogFile: accessLogFile,
	}

	if err := s.Start(t.Context()); err == nil {
		t.Fatal("Start returned nil error for invalid listen address")
	}

	if _, err := accessLogFile.WriteString("after close"); err == nil {
		t.Fatal("access log file is still open after ListenAndServe failure")
	}
	if s.accessLogFile != nil {
		t.Fatal("server still keeps a closed access log file reference")
	}
}

func TestStartClosesAccessLogFileAfterCleanShutdown(t *testing.T) {
	port := freeLocalPort(t)
	accessLogFile, err := os.CreateTemp(t.TempDir(), "access-*.log")
	if err != nil {
		t.Fatalf("create access log: %v", err)
	}
	// The server should close this first; cleanup is only a fallback.
	t.Cleanup(func() { _ = accessLogFile.Close() })

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s := &Server{
		config:        &config.Config{Port: port},
		mux:           http.NewServeMux(),
		accessLogger:  slog.New(slog.NewJSONHandler(accessLogFile, nil)),
		accessLogFile: accessLogFile,
	}
	s.setupRoutes()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Start(ctx)
	}()

	waitForHealth(t, port, errCh)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start returned error after clean shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
	if _, err := accessLogFile.WriteString("after close"); err == nil {
		t.Fatal("access log file is still open after clean shutdown")
	}
	if s.accessLogFile != nil {
		t.Fatal("server still keeps a closed access log file reference")
	}
}

func TestCloseAccessLogFileAllowsNilFile(t *testing.T) {
	s := &Server{}

	s.closeAccessLogFile()

	if s.accessLogFile != nil {
		t.Fatal("nil access log file should remain nil")
	}
}

func TestLoggingResponseWriterCapturesImplicitStatusOnce(t *testing.T) {
	response := httptest.NewRecorder()
	w := &loggingResponseWriter{ResponseWriter: response, statusCode: http.StatusOK}

	if _, err := w.Write([]byte("body")); err != nil {
		t.Fatalf("write response: %v", err)
	}
	w.WriteHeader(http.StatusTeapot)

	if w.statusCode != http.StatusOK {
		t.Fatalf("logged status = %d, want %d", w.statusCode, http.StatusOK)
	}
	if w.Unwrap() != response {
		t.Fatal("Unwrap did not return the underlying response writer")
	}
}

func TestLoggingResponseWriterCapturesFinalStatus(t *testing.T) {
	tests := []struct {
		name       string
		statuses   []int
		wantCodes  []int
		wantStatus int
	}{
		{
			name:       "early hints before final status",
			statuses:   []int{http.StatusEarlyHints, http.StatusNotFound},
			wantCodes:  []int{http.StatusEarlyHints, http.StatusNotFound},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "switching protocols is final",
			statuses:   []int{http.StatusSwitchingProtocols, http.StatusNotFound},
			wantCodes:  []int{http.StatusSwitchingProtocols},
			wantStatus: http.StatusSwitchingProtocols,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &statusCodeRecorder{ResponseRecorder: httptest.NewRecorder()}
			w := &loggingResponseWriter{ResponseWriter: response, statusCode: http.StatusOK}

			for _, status := range tt.statuses {
				w.WriteHeader(status)
			}

			if w.statusCode != tt.wantStatus {
				t.Fatalf("logged status = %d, want %d", w.statusCode, tt.wantStatus)
			}
			if !slices.Equal(response.statusCodes, tt.wantCodes) {
				t.Fatalf("written statuses = %v, want %v", response.statusCodes, tt.wantCodes)
			}
		})
	}
}

type statusCodeRecorder struct {
	*httptest.ResponseRecorder
	statusCodes []int
}

func (r *statusCodeRecorder) WriteHeader(code int) {
	r.statusCodes = append(r.statusCodes, code)
	r.ResponseRecorder.WriteHeader(code)
}

func TestHandleRecordingsServesFileFromRoot(t *testing.T) {
	recordingsDir := t.TempDir()
	const contents = "audio data"
	if err := os.WriteFile(filepath.Join(recordingsDir, "recording.mp3"), []byte(contents), 0o600); err != nil {
		t.Fatalf("write recording: %v", err)
	}

	s := &Server{config: &config.Config{RecordingsDir: recordingsDir}}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/recordings/recording.mp3", nil)
	req.SetPathValue("path", "recording.mp3")
	response := httptest.NewRecorder()

	s.handleRecordings(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Body.String(); got != contents {
		t.Fatalf("body = %q, want %q", got, contents)
	}
}

func TestHandleRecordingsRejectsEscapingPaths(t *testing.T) {
	baseDir := t.TempDir()
	recordingsDir := filepath.Join(baseDir, "recordings")
	if err := os.Mkdir(recordingsDir, 0o700); err != nil {
		t.Fatalf("create recordings directory: %v", err)
	}
	secretPath := filepath.Join(baseDir, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	s := &Server{config: &config.Config{RecordingsDir: recordingsDir}}
	assertRejected := func(t *testing.T, path string) {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/recordings/", nil)
		req.SetPathValue("path", path)
		response := httptest.NewRecorder()

		s.handleRecordings(response, req)

		if response.Code == http.StatusOK {
			t.Fatal("escaping path unexpectedly returned 200 OK")
		}
		if strings.Contains(response.Body.String(), "secret") {
			t.Fatal("response exposed data outside the recordings root")
		}
	}

	t.Run("parent traversal", func(t *testing.T) {
		assertRejected(t, "../secret.txt")
	})
	t.Run("escaping symlink", func(t *testing.T) {
		if err := os.Symlink(secretPath, filepath.Join(recordingsDir, "escape")); err != nil {
			t.Skipf("create symlink: %v", err)
		}
		assertRejected(t, "escape")
	})
}

func freeLocalPort(t *testing.T) int {
	t.Helper()

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on free port: %v", err)
	}
	defer func() { _ = listener.Close() }()

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address has type %T, want *net.TCPAddr", listener.Addr())
	}
	return tcpAddr.Port
}

func waitForHealth(t *testing.T, port int, errCh <-chan error) {
	t.Helper()

	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	client := &http.Client{Timeout: 100 * time.Millisecond}
	for range 50 {
		select {
		case err := <-errCh:
			t.Fatalf("Start returned before server became healthy: %v", err)
		default:
		}

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
		if err != nil {
			t.Fatalf("create health request: %v", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		select {
		case err := <-errCh:
			t.Fatalf("Start returned before server became healthy: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
	}

	t.Fatalf("server did not become healthy at %s", url)
}
