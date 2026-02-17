package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
)

// FileInfo represents a file or directory in the listing.
type FileInfo struct {
	Name    string
	Size    string
	ModTime string
	IsDir   bool
	URL     string
}

// handleRecordings serves files and directory listings from the recordings directory.
func (s *Server) handleRecordings(w http.ResponseWriter, r *http.Request) {
	// Extract the filepath from URL path parameter
	urlPath := r.PathValue("path")
	if urlPath == "" {
		urlPath = "/"
	} else {
		urlPath = "/" + urlPath
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

// showDirectoryListing displays an HTML directory listing.
func (s *Server) showDirectoryListing(w http.ResponseWriter, _ *http.Request, fsPath, urlPath string) {
	// Read directory
	entries, err := os.ReadDir(fsPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}

	// Build file list for directory contents
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
			fileInfo.Size = humanize.Bytes(uint64(info.Size())) //nolint:gosec // File sizes are always non-negative
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

	// Render directory listing using template
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
		slog.Error("failed to execute template", "error", err)
	}
}
