// Package server provides HTTP endpoints for controlling recordings
package server

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/gin-gonic/gin"
	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/postprocessor"
	"github.com/oszuidwest/zwfm-audiologger/internal/recorder"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
)

// Server handles HTTP requests for recording control
type Server struct {
	config          *config.Config
	recorder        *recorder.Manager
	postProcessor   *postprocessor.Manager
	responseBuilder *utils.ResponseBuilder
}

// New creates a new HTTP server
func New(cfg *config.Config, rec *recorder.Manager, pp *postprocessor.Manager) *Server {
	return &Server{
		config:          cfg,
		recorder:        rec,
		postProcessor:   pp,
		responseBuilder: utils.NewResponseBuilder(),
	}
}

// Start begins listening for HTTP requests
func (s *Server) Start() error {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// Program marking endpoints with auth middleware
	auth := r.Group("/")
	auth.Use(s.authMiddleware())
	auth.POST("/program/start/:station", s.handleProgramStart)
	auth.POST("/program/stop/:station", s.handleProgramStop)

	// Public endpoints
	r.GET("/status", s.handleStatus)
	r.GET("/health", s.handleHealth)
	r.GET("/recordings/*filepath", s.handleRecordings)

	log.Printf("HTTP server listening on port %d", s.config.Port)
	log.Printf("Endpoints:")
	log.Printf("  - POST /program/start/:station (requires auth)")
	log.Printf("  - POST /program/stop/:station (requires auth)")
	log.Printf("  - GET /recordings/* (browse recordings)")
	log.Printf("  - GET /status (system status)")
	log.Printf("  - GET /health (health check)")

	return r.Run(fmt.Sprintf(":%d", s.config.Port))
}

// handleStatus handles requests for recording status
func (s *Server) handleStatus(c *gin.Context) {
	activeRecordings := s.recorder.ActiveRecordings()

	status := make(map[string]interface{})
	for name, rec := range activeRecordings {
		status[name] = gin.H{
			"recording": true,
			"started":   rec.StartTime.Format(time.RFC3339),
			"duration":  utils.Now().Sub(rec.StartTime).String(),
		}
	}

	c.JSON(http.StatusOK, status)
}

// handleHealth handles health check requests
func (s *Server) handleHealth(c *gin.Context) {
	c.String(http.StatusOK, "OK")
}

// handleProgramStart marks when a program starts (commercials end)
func (s *Server) handleProgramStart(c *gin.Context) {
	station := c.Param("station")
	if station == "" {
		s.responseBuilder.BadRequest(c, "Station name required")
		return
	}

	s.postProcessor.MarkProgramStart(station)
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Marked program start for %s", station)})
}

// handleProgramStop marks when a program ends (commercials start)
func (s *Server) handleProgramStop(c *gin.Context) {
	station := c.Param("station")
	if station == "" {
		s.responseBuilder.BadRequest(c, "Station name required")
		return
	}

	s.postProcessor.MarkProgramEnd(station)
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Marked program end for %s", station)})
}

// authMiddleware provides authentication middleware for Gin
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get the station from the URL parameter
		station := c.Param("station")

		// Check if station exists in config
		stationConfig, exists := s.config.Stations[station]
		if !exists {
			s.responseBuilder.NotFound(c, "Unknown station")
			c.Abort()
			return
		}

		// Require station-specific API secret
		expectedSecret := stationConfig.APISecret
		if expectedSecret == "" {
			s.responseBuilder.Unauthorized(c)
			c.Abort()
			return
		}

		// Check Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			// Support "Bearer <secret>" format
			if strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				if token == expectedSecret {
					c.Next()
					return
				}
			}
			// Also support just the secret directly
			if authHeader == expectedSecret {
				c.Next()
				return
			}
		}

		// Check X-API-Key header
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == expectedSecret {
			c.Next()
			return
		}

		// Check query parameter as fallback
		secret := c.Query("secret")
		if secret == expectedSecret {
			c.Next()
			return
		}

		s.responseBuilder.Unauthorized(c)
		c.Abort()
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
func (s *Server) handleRecordings(c *gin.Context) {
	// Get the filepath parameter
	urlPath := c.Param("filepath")
	if urlPath == "" || urlPath == "/" {
		urlPath = "/"
	}

	// Construct the filesystem path
	fsPath := filepath.Join(s.config.RecordingsDir, filepath.Clean(urlPath))

	// Get file info
	info, err := os.Stat(fsPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.responseBuilder.NotFound(c, "File not found")
			return
		}
		s.responseBuilder.InternalError(c, err.Error())
		return
	}

	// If it's a file, serve it
	if !info.IsDir() {
		// Set content type based on file extension
		ext := filepath.Ext(fsPath)
		switch ext {
		case ".meta":
			c.Header("Content-Type", "text/plain; charset=utf-8")
		case ".json":
			c.Header("Content-Type", "application/json")
		default:
			// Use the format utility for audio files
			contentType := utils.ContentType(ext)
			c.Header("Content-Type", contentType)
		}

		// Set Content-Disposition for download
		c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", path.Base(fsPath)))

		// Serve the file
		c.File(fsPath)
		return
	}

	// It's a directory, show listing
	s.showDirectoryListing(c, fsPath, urlPath)
}

// showDirectoryListing displays an HTML directory listing
func (s *Server) showDirectoryListing(c *gin.Context, fsPath, urlPath string) {
	// Read directory
	entries, err := os.ReadDir(fsPath)
	if err != nil {
		s.responseBuilder.InternalError(c, err.Error())
		return
	}

	// Build file list
	var files []FileInfo

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
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
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

	// Render HTML
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
	if err != nil {
		s.responseBuilder.InternalError(c, err.Error())
		return
	}

	data := struct {
		Path  string
		Files []FileInfo
	}{
		Path:  urlPath,
		Files: files,
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(c.Writer, data); err != nil {
		log.Printf("Failed to execute template: %v", err)
	}
}
