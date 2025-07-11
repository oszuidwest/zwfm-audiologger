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

type StreamInfo struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Status      string `json:"status"`
	LastSeen    string `json:"last_seen,omitempty"`
	Recordings  int    `json:"recordings_count"`
	TotalSize   int64  `json:"total_size_bytes"`
	KeepDays    int    `json:"keep_days"`
	HasMetadata bool   `json:"has_metadata"`
}

type StreamsResponse struct {
	Streams []StreamInfo `json:"streams"`
}

type Recording struct {
	Timestamp   string        `json:"timestamp"`
	StartTime   string        `json:"start_time"`
	EndTime     string        `json:"end_time"`
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

type MetadataResponse struct {
	Stream    string `json:"stream"`
	Timestamp string `json:"timestamp"`
	Metadata  string `json:"metadata"`
	FetchedAt string `json:"fetched_at"`
}

type SystemStats struct {
	Uptime          string                `json:"uptime"`
	TotalRecordings int                   `json:"total_recordings"`
	TotalSize       int64                 `json:"total_size_bytes"`
	StreamStats     map[string]StreamStat `json:"stream_stats"`
	CacheStats      interface{}           `json:"cache_stats"`
}

type StreamStat struct {
	Recordings int       `json:"recordings"`
	SizeBytes  int64     `json:"size_bytes"`
	LastActive time.Time `json:"last_active"`
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("failed to encode JSON response", "error", err)
	}
}

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

func (s *Server) calculateStreamStats(streamName string) (totalSize int64, lastSeen time.Time, recordingCount int, err error) {
	recordings, err := s.getRecordings(streamName)
	if err != nil {
		return 0, time.Time{}, 0, err
	}

	for _, recording := range recordings {
		totalSize += recording.Size
		if t, err := utils.ParseTimestamp(recording.Timestamp, s.config.Timezone); err == nil {
			if t.After(lastSeen) {
				lastSeen = t
			}
		}
	}

	return totalSize, lastSeen, len(recordings), nil
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(startTime).String()

	s.writeJSON(w, http.StatusOK, HealthResponse{
		Status:    "ok",
		Timestamp: time.Now(),
		Version:   "1.0.0",
		Uptime:    uptime,
	})
}

func (s *Server) readinessHandler(w http.ResponseWriter, r *http.Request) {
	checks := []Check{
		{Name: "cache", Status: "ok"},
		{Name: "storage", Status: "ok"},
	}

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

func (s *Server) streamDetailsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	streamName := vars["stream"]

	stream, exists := s.config.Streams[streamName]
	if !exists {
		s.writeAPIError(w, http.StatusNotFound, "Stream not found", fmt.Sprintf("Stream '%s' does not exist", streamName))
		return
	}

	totalSize, lastSeen, recordingCount, _ := s.calculateStreamStats(streamName)

	streamInfo := StreamInfo{
		Name:        streamName,
		URL:         stream.URL,
		Status:      "active",
		LastSeen:    utils.ToAPIString(lastSeen, s.config.Timezone),
		Recordings:  recordingCount,
		TotalSize:   totalSize,
		KeepDays:    s.config.GetStreamKeepDays(streamName),
		HasMetadata: stream.MetadataURL != "",
	}

	s.writeAPIResponse(w, http.StatusOK, streamInfo, 1)
}

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

	startTime, err := utils.ParseTimestamp(timestamp, s.config.Timezone)
	if err != nil {
		s.writeAPIError(w, http.StatusBadRequest, "Invalid timestamp format", err.Error())
		return
	}
	endTime := startTime.Add(time.Hour)

	metadataPath := utils.MetadataPath(s.config.RecordingDir, streamName, timestamp)
	hasMetadata := utils.FileExists(metadataPath)

	baseURL := fmt.Sprintf("/api/v1/streams/%s/recordings/%s", streamName, timestamp)

	recording := Recording{
		Timestamp:   timestamp,
		StartTime:   utils.ToAPIString(startTime, s.config.Timezone),
		EndTime:     utils.ToAPIString(endTime, s.config.Timezone),
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

func (s *Server) streamsHandler(w http.ResponseWriter, r *http.Request) {
	streams := make([]StreamInfo, 0, len(s.config.Streams))

	for streamName, stream := range s.config.Streams {
		totalSize, lastSeen, recordingCount, _ := s.calculateStreamStats(streamName)

		streamInfo := StreamInfo{
			Name:        streamName,
			URL:         stream.URL,
			Status:      "active",
			LastSeen:    utils.ToAPIString(lastSeen, s.config.Timezone),
			Recordings:  recordingCount,
			TotalSize:   totalSize,
			KeepDays:    s.config.GetStreamKeepDays(streamName),
			HasMetadata: stream.MetadataURL != "",
		}

		streams = append(streams, streamInfo)
	}

	s.writeAPIResponse(w, http.StatusOK, StreamsResponse{Streams: streams}, len(streams))
}

func (s *Server) recordingsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	streamName := vars["stream"]

	if _, exists := s.config.Streams[streamName]; !exists {
		s.writeAPIError(w, http.StatusNotFound, "Stream not found", fmt.Sprintf("Stream '%s' does not exist", streamName))
		return
	}

	rawRecordings, err := s.getRecordings(streamName)
	if err != nil {
		s.logger.Error("failed to list recordings", "error", err)
		s.writeAPIError(w, http.StatusInternalServerError, "Failed to list recordings", err.Error())
		return
	}

	recordings := make([]Recording, len(rawRecordings))
	for i, raw := range rawRecordings {
		startTime, _ := utils.ParseTimestamp(raw.Timestamp, s.config.Timezone)
		endTime := startTime.Add(time.Hour)

		metadataPath := utils.MetadataPath(s.config.RecordingDir, streamName, raw.Timestamp)
		hasMetadata := utils.FileExists(metadataPath)

		baseURL := fmt.Sprintf("/api/v1/streams/%s/recordings/%s", streamName, raw.Timestamp)

		recordings[i] = Recording{
			Timestamp:   raw.Timestamp,
			StartTime:   utils.ToAPIString(startTime, s.config.Timezone),
			EndTime:     utils.ToAPIString(endTime, s.config.Timezone),
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
		FetchedAt: utils.ToAPIString(utils.NowInTimezone(s.config.Timezone), s.config.Timezone),
	}

	s.writeAPIResponse(w, http.StatusOK, response, 1)
}

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
