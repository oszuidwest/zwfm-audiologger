package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
)

// API Response structures
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
	Meta    *APIMeta    `json:"meta,omitempty"`
}

type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

type APIMeta struct {
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version,omitempty"`
	Count     int       `json:"count,omitempty"`
}

// Health check responses
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
	Uptime    string    `json:"uptime,omitempty"`
}

type ReadinessResponse struct {
	Ready   bool    `json:"ready"`
	Checks  []Check `json:"checks"`
	Message string  `json:"message,omitempty"`
}

type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// Stream responses
type StreamInfo struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Status      string `json:"status"`
	LastSeen    string `json:"last_seen,omitempty"`    // YYYY-MM-DD HH:MM format (API)
	Recordings  int    `json:"recordings_count"`
	TotalSize   int64  `json:"total_size_bytes"`
	KeepDays    int    `json:"keep_days"`
	HasMetadata bool   `json:"has_metadata"`
}

type StreamsResponse struct {
	Streams []StreamInfo `json:"streams"`
}

// Recording responses
type Recording struct {
	Timestamp   string        `json:"timestamp"`           // YYYY-MM-DD-HH format (universal)
	StartTime   string        `json:"start_time"`          // YYYY-MM-DD HH:MM format (API)
	EndTime     string        `json:"end_time"`            // YYYY-MM-DD HH:MM format (API)
	Duration    string        `json:"duration"`
	Size        int64         `json:"size_bytes"`
	SizeHuman   string        `json:"size_human"`
	HasMetadata bool          `json:"has_metadata"`
	URLs        RecordingURLs `json:"urls"`
}

type RecordingURLs struct {
	Download string `json:"download"`
	Stream   string `json:"stream"`
	Metadata string `json:"metadata,omitempty"`
}

type RecordingsResponse struct {
	Recordings []Recording `json:"recordings"`
	Stream     string      `json:"stream"`
}

// Metadata responses
type MetadataResponse struct {
	Stream    string `json:"stream"`
	Timestamp string `json:"timestamp"`     // YYYY-MM-DD-HH format (universal)
	Metadata  string `json:"metadata"`
	FetchedAt string `json:"fetched_at"`    // YYYY-MM-DD HH:MM format (API)
}

// System responses
type SystemStats struct {
	Uptime          string                    `json:"uptime"`
	TotalRecordings int                       `json:"total_recordings"`
	TotalSize       int64                     `json:"total_size_bytes"`
	StreamStats     map[string]StreamStat     `json:"stream_stats"`
	CacheStats      interface{}               `json:"cache_stats"`
}

type StreamStat struct {
	Recordings int       `json:"recordings"`
	SizeBytes  int64     `json:"size_bytes"`
	LastActive time.Time `json:"last_active"`
}

// writeJSON writes a JSON response
func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("Failed to encode JSON response: ", err)
	}
}

// writeAPIResponse writes a structured API response
func (s *Server) writeAPIResponse(w http.ResponseWriter, status int, data interface{}, count int) {
	response := APIResponse{
		Success: status < 400,
		Data:    data,
		Meta: &APIMeta{
			Timestamp: time.Now(),
			Version:   "1.0.0",
			Count:     count,
		},
	}
	
	if status >= 400 {
		response.Error = &APIError{
			Code:    status,
			Message: http.StatusText(status),
		}
	}
	
	s.writeJSON(w, status, response)
}

// writeAPIError writes a structured API error response  
func (s *Server) writeAPIError(w http.ResponseWriter, status int, message, details string) {
	response := APIResponse{
		Success: false,
		Error: &APIError{
			Code:    status,
			Message: message,
			Details: details,
		},
		Meta: &APIMeta{
			Timestamp: time.Now(),
			Version:   "1.0.0",
		},
	}
	
	s.writeJSON(w, status, response)
}


var startTime = time.Now()

// calculateStreamStats calculates statistics for a stream
func (s *Server) calculateStreamStats(streamName string) (totalSize int64, lastSeen time.Time, recordingCount int, err error) {
	recordings, err := s.getRecordings(streamName)
	if err != nil {
		return 0, time.Time{}, 0, err
	}

	for _, recording := range recordings {
		totalSize += recording.Size
		if t, err := utils.ParseTimestamp(recording.Timestamp); err == nil {
			if t.After(lastSeen) {
				lastSeen = t
			}
		}
	}

	return totalSize, lastSeen, len(recordings), nil
}

// healthHandler returns basic health information
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(startTime).String()
	
	s.writeJSON(w, http.StatusOK, HealthResponse{
		Status:    "ok",
		Timestamp: time.Now(),
		Version:   "1.0.0",
		Uptime:    uptime,
	})
}

// readinessHandler checks if the service is ready to serve requests
func (s *Server) readinessHandler(w http.ResponseWriter, r *http.Request) {
	checks := []Check{
		{Name: "cache", Status: "ok"},
		{Name: "storage", Status: "ok"},
	}
	
	// Check if recording directory is accessible
	if _, err := os.Stat(s.config.RecordingDir); os.IsNotExist(err) {
		checks[1].Status = "error"
		checks[1].Message = "Recording directory not accessible"
	}
	
	allReady := true
	for _, check := range checks {
		if check.Status != "ok" {
			allReady = false
			break
		}
	}
	
	status := http.StatusOK
	if !allReady {
		status = http.StatusServiceUnavailable
	}
	
	s.writeAPIResponse(w, status, ReadinessResponse{
		Ready:  allReady,
		Checks: checks,
	}, len(checks))
}

// streamDetailsHandler returns detailed information about a specific stream
func (s *Server) streamDetailsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	streamName := vars["stream"]
	
	stream, exists := s.config.Streams[streamName]
	if !exists {
		s.writeAPIError(w, http.StatusNotFound, "Stream not found", fmt.Sprintf("Stream '%s' does not exist", streamName))
		return
	}
	
	// Get stream statistics
	totalSize, lastSeen, recordingCount, _ := s.calculateStreamStats(streamName)
	
	streamInfo := StreamInfo{
		Name:        streamName,
		URL:         stream.URL,
		Status:      "active",
		LastSeen:    utils.ToAPIString(lastSeen),
		Recordings:  recordingCount,
		TotalSize:   totalSize,
		KeepDays:    s.config.GetStreamKeepDays(streamName),
		HasMetadata: stream.MetadataURL != "",
	}
	
	s.writeAPIResponse(w, http.StatusOK, streamInfo, 1)
}

// recordingHandler returns information about a specific recording
func (s *Server) recordingHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	streamName := vars["stream"]
	timestamp := vars["timestamp"]
	
	if _, exists := s.config.Streams[streamName]; !exists {
		s.writeAPIError(w, http.StatusNotFound, "Stream not found", fmt.Sprintf("Stream '%s' does not exist", streamName))
		return
	}
	
	recordingPath := utils.RecordingPath(s.config.RecordingDir, streamName, timestamp)
	
	if !utils.FileExists(recordingPath) {
		s.writeAPIError(w, http.StatusNotFound, "Recording not found", fmt.Sprintf("Recording '%s' does not exist", timestamp))
		return
	}
	
	stat, err := os.Stat(recordingPath)
	if err != nil {
		s.writeAPIError(w, http.StatusInternalServerError, "Failed to get recording info", err.Error())
		return
	}
	
	// Parse timestamp to get start/end times
	startTime, err := utils.ParseTimestamp(timestamp)
	if err != nil {
		s.writeAPIError(w, http.StatusBadRequest, "Invalid timestamp format", err.Error())
		return
	}
	endTime := startTime.Add(time.Hour)
	
	// Check if metadata exists
	metadataPath := utils.MetadataPath(s.config.RecordingDir, streamName, timestamp)
	hasMetadata := utils.FileExists(metadataPath)
	
	baseURL := fmt.Sprintf("/api/v1/streams/%s/recordings/%s", streamName, timestamp)
	
	recording := Recording{
		Timestamp:   timestamp,
		StartTime:   utils.ToAPIString(startTime),
		EndTime:     utils.ToAPIString(endTime),
		Duration:    "1h",
		Size:        stat.Size(),
		SizeHuman:   formatFileSize(stat.Size()),
		HasMetadata: hasMetadata,
		URLs: RecordingURLs{
			Download: baseURL + "/download",
			Stream:   baseURL,
			Metadata: func() string {
				if hasMetadata {
					return baseURL + "/metadata"
				}
				return ""
			}(),
		},
	}
	
	s.writeAPIResponse(w, http.StatusOK, recording, 1)
}

// downloadRecordingHandler serves a complete recording file for download
func (s *Server) downloadRecordingHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	streamName := vars["stream"]
	timestamp := vars["timestamp"]
	
	if _, exists := s.config.Streams[streamName]; !exists {
		s.writeAPIError(w, http.StatusNotFound, "Stream not found", fmt.Sprintf("Stream '%s' does not exist", streamName))
		return
	}
	
	recordingPath := utils.RecordingPath(s.config.RecordingDir, streamName, timestamp)
	
	if !utils.FileExists(recordingPath) {
		s.writeAPIError(w, http.StatusNotFound, "Recording not found", fmt.Sprintf("Recording '%s' does not exist", timestamp))
		return
	}
	
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_%s.mp3\"", streamName, timestamp))
	
	http.ServeFile(w, r, recordingPath)
}

// systemStatsHandler returns comprehensive system statistics
func (s *Server) systemStatsHandler(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(startTime).String()
	
	streamStats := make(map[string]StreamStat)
	totalRecordings := 0
	totalSize := int64(0)
	
	for streamName := range s.config.Streams {
		streamSize, lastActive, recordingCount, _ := s.calculateStreamStats(streamName)
		
		streamStats[streamName] = StreamStat{
			Recordings: recordingCount,
			SizeBytes:  streamSize,
			LastActive: lastActive,
		}
		
		totalRecordings += recordingCount
		totalSize += streamSize
	}
	
	stats := SystemStats{
		Uptime:          uptime,
		TotalRecordings: totalRecordings,
		TotalSize:       totalSize,
		StreamStats:     streamStats,
		CacheStats:      s.cache.GetCacheStats(),
	}
	
	s.writeAPIResponse(w, http.StatusOK, stats, 1)
}

// streamsHandler returns detailed information about all streams
func (s *Server) streamsHandler(w http.ResponseWriter, r *http.Request) {
	streams := make([]StreamInfo, 0, len(s.config.Streams))
	
	for streamName, stream := range s.config.Streams {
		totalSize, lastSeen, recordingCount, _ := s.calculateStreamStats(streamName)
		
		streamInfo := StreamInfo{
			Name:        streamName,
			URL:         stream.URL,
			Status:      "active",
			LastSeen:    utils.ToAPIString(lastSeen),
			Recordings:  recordingCount,
			TotalSize:   totalSize,
			KeepDays:    s.config.GetStreamKeepDays(streamName),
			HasMetadata: stream.MetadataURL != "",
		}
		
		streams = append(streams, streamInfo)
	}
	
	s.writeAPIResponse(w, http.StatusOK, StreamsResponse{Streams: streams}, len(streams))
}

// recordingsHandler returns detailed information about recordings for a stream
func (s *Server) recordingsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	streamName := vars["stream"]
	
	if _, exists := s.config.Streams[streamName]; !exists {
		s.writeAPIError(w, http.StatusNotFound, "Stream not found", fmt.Sprintf("Stream '%s' does not exist", streamName))
		return
	}
	
	rawRecordings, err := s.getRecordings(streamName)
	if err != nil {
		s.logger.Error("Failed to list recordings: ", err)
		s.writeAPIError(w, http.StatusInternalServerError, "Failed to list recordings", err.Error())
		return
	}
	
	// Convert to enhanced recordings
	recordings := make([]Recording, len(rawRecordings))
	for i, raw := range rawRecordings {
		startTime, _ := utils.ParseTimestamp(raw.Timestamp)
		endTime := startTime.Add(time.Hour)
		
		// Check if metadata exists
		metadataPath := utils.MetadataPath(s.config.RecordingDir, streamName, raw.Timestamp)
		hasMetadata := utils.FileExists(metadataPath)
		
		baseURL := fmt.Sprintf("/api/v1/streams/%s/recordings/%s", streamName, raw.Timestamp)
		
		recordings[i] = Recording{
			Timestamp:   raw.Timestamp,
			StartTime:   utils.ToAPIString(startTime),
			EndTime:     utils.ToAPIString(endTime),
			Duration:    "1h",
			Size:        raw.Size,
			SizeHuman:   formatFileSize(raw.Size),
			HasMetadata: hasMetadata,
			URLs: RecordingURLs{
				Download: baseURL + "/download",
				Stream:   baseURL,
				Metadata: func() string {
					if hasMetadata {
						return baseURL + "/metadata"
					}
					return ""
				}(),
			},
		}
	}
	
	response := RecordingsResponse{
		Recordings: recordings,
		Stream:     streamName,
	}
	
	s.writeAPIResponse(w, http.StatusOK, response, len(recordings))
}

// metadataHandler returns metadata for a specific recording
func (s *Server) metadataHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	streamName := vars["stream"]
	timestamp := vars["timestamp"]
	
	if _, exists := s.config.Streams[streamName]; !exists {
		s.writeAPIError(w, http.StatusNotFound, "Stream not found", fmt.Sprintf("Stream '%s' does not exist", streamName))
		return
	}
	
	metadata, err := s.metadata.GetMetadata(utils.StreamDir(s.config.RecordingDir, streamName), timestamp)
	if err != nil {
		s.writeAPIError(w, http.StatusNotFound, "Metadata not found", err.Error())
		return
	}
	
	response := MetadataResponse{
		Stream:    streamName,
		Timestamp: timestamp,
		Metadata:  metadata,
		FetchedAt: utils.ToAPIString(utils.Now()),
	}
	
	s.writeAPIResponse(w, http.StatusOK, response, 1)
}

// Utility functions
func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}