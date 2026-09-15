package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
)

// fileInfo represents a file or directory in the listing.
type fileInfo struct {
	Name    string
	Size    string
	ModTime string
	IsDir   bool
	URL     string
}

// extensionContentType returns the content type for a file extension.
func extensionContentType(ext string) string {
	switch ext {
	case ".meta":
		return "text/plain; charset=utf-8"
	case ".json":
		return "application/json"
	default:
		return utils.ContentType(ext)
	}
}

// handleRecordings serves files and directory listings from the recordings directory.
func (s *Server) handleRecordings(w http.ResponseWriter, r *http.Request) {
	// Clean drops the trailing "/" of directory links and clamps ".." to the root.
	urlPath := path.Clean("/" + r.PathValue("path"))

	root, err := os.OpenRoot(s.config.RecordingsDir)
	if err != nil {
		slog.Error("failed to open recordings root", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}
	defer func() { _ = root.Close() }()

	file, err := root.Open(path.Join(".", urlPath))
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("refused recording path", "path", urlPath, "error", err)
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "File not found"})
		return
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		slog.Error("failed to stat recording path", "path", urlPath, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}

	if !info.IsDir() {
		w.Header().Set("Content-Type", extensionContentType(filepath.Ext(info.Name())))
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", info.Name()))
		http.ServeContent(w, r, info.Name(), info.ModTime(), file)
		return
	}

	s.showDirectoryListing(w, file, urlPath)
}

// showDirectoryListing displays an HTML directory listing.
func (s *Server) showDirectoryListing(w http.ResponseWriter, dir *os.File, urlPath string) {
	entries, err := dir.ReadDir(-1)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}

	// Build file list for directory contents
	capacity := len(entries)
	if urlPath != "/" {
		capacity++ // Add space for parent directory
	}
	files := make([]fileInfo, 0, capacity)

	// Add parent directory link if not at root
	if urlPath != "/" {
		files = append(files, fileInfo{
			Name:  "../",
			IsDir: true,
			URL:   path.Dir("/recordings"+urlPath) + "/",
		})
	}

	// Process entries
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			slog.Warn(
				"failed to read directory entry info, skipping",
				"entry", entry.Name(),
				"error", err,
			)
			continue
		}

		fi := fileInfo{
			Name:    entry.Name(),
			IsDir:   entry.IsDir(),
			ModTime: info.ModTime().Format(time.DateTime),
		}

		if entry.IsDir() {
			fi.Name += "/"
			fi.URL = "/recordings" + path.Join(urlPath, entry.Name()) + "/"
			fi.Size = "-"
		} else {
			fi.URL = "/recordings" + path.Join(urlPath, entry.Name())
			fi.Size = humanize.Bytes(uint64(info.Size())) //nolint:gosec // File sizes are always non-negative
		}

		files = append(files, fi)
	}

	// Sort files (directories first, then by name)
	slices.SortFunc(files, func(a, b fileInfo) int {
		if a.IsDir != b.IsDir {
			if a.IsDir {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})

	// Render directory listing using template
	t := directoryTemplate()

	data := struct {
		Path  string
		Files []fileInfo
	}{
		Path:  urlPath,
		Files: files,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, data); err != nil {
		slog.Error("failed to execute template", "error", err)
	}
}
