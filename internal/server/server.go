// Package server provides HTTP endpoints for controlling recordings
package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/postprocessor"
	"github.com/oszuidwest/zwfm-audiologger/internal/recorder"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
)

// directoryTemplate uses sync.OnceValue for efficient template compilation (Go 1.21+)
var directoryTemplate = sync.OnceValue(func() *template.Template {
	tmpl := `<!DOCTYPE html>
<html>
<head>
    <title>Recordings - {{.Path}}</title>
    <style>
        body { font-family: monospace; margin: 20px; }
        h1 { font-size: 24px; }
        table { border-collapse: collapse; width: 100%; max-width: 1000px; }
        th { text-align: left; border-bottom: 1px solid #ddd; padding: 8px; }
        td { padding: 8px; }
        tr:hover { background-color: #f5f5f5; }
        a { text-decoration: none; color: #0066cc; }
        a:hover { text-decoration: underline; }
        .size { text-align: right; }
        .time { color: #666; }
    </style>
</head>
<body>
    <h1>Index of /recordings{{.Path}}</h1>
    <table>
        <thead>
            <tr>
                <th>Name</th>
                <th>Size</th>
                <th>Modified</th>
            </tr>
        </thead>
        <tbody>
            {{range .Files}}
            <tr>
                <td><a href="{{.URL}}">{{.Name}}</a></td>
                <td class="size">{{.Size}}</td>
                <td class="time">{{.ModTime}}</td>
            </tr>
            {{end}}
        </tbody>
    </table>
</body>
</html>`

	t, err := template.New("listing").Parse(tmpl)
		panic(fmt.Sprintf("template parse error: %v", err))
	}
	return t
})

// Server handles HTTP requests for recording control
type Server struct {
	config        *config.Config
	recorder      *recorder.Manager
	postProcessor *postprocessor.Manager
	mux           *http.ServeMux
}

// New creates a new HTTP server
func New(cfg *config.Config, rec *recorder.Manager, pp *postprocessor.Manager) *Server {
	s := &Server{
		config:        cfg,
		recorder:      rec,
		postProcessor: pp,
		mux:           http.NewServeMux(),
	}

	// Setup routes
	s.setupRoutes()

	return s
}

// setupRoutes configures the HTTP routes
func (s *Server) setupRoutes() {
	// Public endpoints
	s.mux.HandleFunc("/status", s.handleStatus)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/recordings/", s.handleRecordings)

	// Protected endpoints with authentication
	s.mux.HandleFunc("/program/start/", s.authenticate(s.handleProgramStart))
	s.mux.HandleFunc("/program/stop/", s.authenticate(s.handleProgramStop))
}

// Start begins listening for HTTP requests
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.config.Port)

	log.Printf("HTTP server listening on port %d", s.config.Port)
	log.Printf("Endpoints:")
	log.Printf("  - POST /program/start/:station (requires auth)")
	log.Printf("  - POST /program/stop/:station (requires auth)")
	log.Printf("  - GET /recordings/* (browse recordings)")
	log.Printf("  - GET /status (system status)")
	log.Printf("  - GET /health (health check)")

	// Create HTTP server with logging middleware
	server := &http.Server{
		Addr:    addr,
		Handler: s.loggingMiddleware(s.mux),
	}

	return server.ListenAndServe()
}

// loggingMiddleware logs HTTP requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap ResponseWriter to capture status code
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(lrw, r)

		log.Printf("%s %s %d %v", r.Method, r.URL.Path, lrw.statusCode, time.Since(start))
	})
}

// loggingResponseWriter wraps http.ResponseWriter to capture status code for logging purposes.
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int // HTTP status code returned by the handler
}

// WriteHeader captures the status code and calls the underlying ResponseWriter's WriteHeader.
func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}

// extractStation extracts station name from URL path using efficient string parsing
func extractStation(path string) string {
	// Use strings.Cut for more efficient parsing (Go 1.18+)
	path = strings.Trim(path, "/")

	// Use strings.Cut to split efficiently
	prefix, rest, found := strings.Cut(path, "/")
	if !found || prefix != "program" {
		return ""
	}

	// Extract the second part (action/station)
	_, station, found := strings.Cut(rest, "/")
	if !found {
		return ""
	}

	return station
}

// handleStatus handles requests for recording status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Return basic system status - recordings are scheduled, not tracked in memory
	status := map[string]interface{}{
		"message": "System running - recordings scheduled hourly",
		"time":    utils.Now().Format(time.RFC3339),
	}

	writeJSON(w, http.StatusOK, status)
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// handleProgramStart marks when a program starts (commercials end)
func (s *Server) handleProgramStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	station := extractStation(r.URL.Path)
	if station == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Station name required"})
		return
	}

	s.postProcessor.MarkProgramStart(station)
	writeJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Marked program start for %s", station)})
}

// handleProgramStop marks when a program ends (commercials start)
func (s *Server) handleProgramStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	station := extractStation(r.URL.Path)
	if station == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Station name required"})
		return
	}

	s.postProcessor.MarkProgramEnd(station)
	writeJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Marked program end for %s", station)})
}

// authenticate provides simple authentication middleware
func (s *Server) authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		station := extractStation(r.URL.Path)

		if station == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Station name required"})
			return
		}

		// Check if station exists in config
		stationConfig, exists := s.config.Stations[station]
		if !exists {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Unknown station"})
			return
		}

		// Simple API key check
		expectedSecret := stationConfig.APISecret
		if expectedSecret == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "No API secret configured"})
			return
		}

		// Check X-API-Key header (most common pattern)
		if r.Header.Get("X-API-Key") == expectedSecret {
			next(w, r)
			return
		}

		// Check Authorization header with Bearer token
		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			if token, found := strings.CutPrefix(authHeader, "Bearer "); found {
				if token == expectedSecret {
					next(w, r)
					return
				}
			}
		}

		// Check query parameter as fallback for simple curl commands
		if r.URL.Query().Get("secret") == expectedSecret {
			next(w, r)
			return
		}

		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API key"})
	}
}

// FileInfo represents a file or directory in the listing
type FileInfo struct {
	Name    string
	Size    string
	ModTime string
	IsDir   bool
	URL     string
}

// handleRecordings serves files and directory listings from the recordings directory
func (s *Server) handleRecordings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract the filepath from URL path
	urlPath := strings.TrimPrefix(r.URL.Path, "/recordings")
	if urlPath == "" {
		urlPath = "/"
	}

	// Simple path construction - recordings are controlled by the system
	fsPath := filepath.Join(s.config.RecordingsDir, filepath.Clean(urlPath))

	// Get file info
	info, err := os.Stat(fsPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "File not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}

	// If it's a file, serve it
	if !info.IsDir() {
		// Set content type based on file extension
		ext := filepath.Ext(fsPath)
		switch ext {
		case ".meta":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		case ".json":
			w.Header().Set("Content-Type", "application/json")
		default:
			// Use the format utility for audio files
			contentType := utils.ContentType(ext)
			w.Header().Set("Content-Type", contentType)
		}

		// Set Content-Disposition for download
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", path.Base(fsPath)))

		// Serve the file
		http.ServeFile(w, r, fsPath)
		return
	}

	// It's a directory, show listing
	s.showDirectoryListing(w, r, fsPath, urlPath)
}

// showDirectoryListing displays an HTML directory listing
func (s *Server) showDirectoryListing(w http.ResponseWriter, r *http.Request, fsPath, urlPath string) {
	// Read directory
	entries, err := os.ReadDir(fsPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}

	// Build file list with pre-allocated slice for better memory efficiency (Go 1.25 optimization)
	// Pre-allocate with known capacity: entries + potential parent directory
	capacity := len(entries)
	if urlPath != "/" {
		capacity++ // Add space for parent directory
	}
	files := make([]FileInfo, 0, capacity)

	// Add parent directory link if not at root
	if urlPath != "/" {
		files = append(files, FileInfo{
			Name:  "../",
			IsDir: true,
			URL:   path.Dir("/recordings"+urlPath) + "/",
		})
	}

	// Process entries
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		fileInfo := FileInfo{
			Name:    entry.Name(),
			IsDir:   entry.IsDir(),
			ModTime: info.ModTime().Format(time.DateTime),
		}

		if entry.IsDir() {
			fileInfo.Name += "/"
			fileInfo.URL = "/recordings" + path.Join(urlPath, entry.Name()) + "/"
			fileInfo.Size = "-"
		} else {
			fileInfo.URL = "/recordings" + path.Join(urlPath, entry.Name())
			fileInfo.Size = humanize.Bytes(uint64(info.Size()))
		}

		files = append(files, fileInfo)
	}

	// Sort files (directories first, then by name)
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return files[i].Name < files[j].Name
	})

	// Use pre-compiled template for better performance
	t := directoryTemplate()

	data := struct {
		Path  string
		Files []FileInfo
	}{
		Path:  urlPath,
		Files: files,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, data); err != nil {
		log.Printf("Failed to execute template: %v", err)
	}
}
