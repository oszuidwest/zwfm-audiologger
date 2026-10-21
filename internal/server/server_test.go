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

func TestLoggingResponseWriterStatus(t *testing.T) {
	tests := []struct {
		name string
		run  func(w *loggingResponseWriter)
		want int
	}{
		{
			name: "explicit status",
			run:  func(w *loggingResponseWriter) { w.WriteHeader(http.StatusNotFound) },
			want: http.StatusNotFound,
		},
		{
			name: "implicit 200 wins over a later WriteHeader",
			run: func(w *loggingResponseWriter) {
				_, _ = w.Write([]byte("body"))
				w.WriteHeader(http.StatusInternalServerError)
			},
			want: http.StatusOK,
		},
		{
			name: "first final status wins over a duplicate",
			run: func(w *loggingResponseWriter) {
				w.WriteHeader(http.StatusNotFound)
				w.WriteHeader(http.StatusTeapot)
			},
			want: http.StatusNotFound,
		},
		{
			name: "early hints do not end the header",
			run: func(w *loggingResponseWriter) {
				w.WriteHeader(http.StatusEarlyHints)
				w.WriteHeader(http.StatusNotFound)
			},
			want: http.StatusNotFound,
		},
		{
			name: "switching protocols is final",
			run: func(w *loggingResponseWriter) {
				w.WriteHeader(http.StatusSwitchingProtocols)
				w.WriteHeader(http.StatusNotFound)
			},
			want: http.StatusSwitchingProtocols,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			w := &loggingResponseWriter{ResponseWriter: response}

			tt.run(w)

			if w.statusCode != tt.want {
				t.Fatalf("logged status = %d, want %d", w.statusCode, tt.want)
			}
			if w.Unwrap() != response {
				t.Fatal("Unwrap did not return the underlying response writer")
			}
		})
	}
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
