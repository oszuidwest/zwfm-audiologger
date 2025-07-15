package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
	"github.com/oszuidwest/zwfm-audiologger/internal/version"
)

const APIVersion = "1.0.0"

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

type StationInfo struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	Status       string `json:"status"`
	LastRecorded string `json:"last_recorded,omitempty"`
	Recordings   int    `json:"recordings_count"`
	TotalSize    int64  `json:"total_size_bytes"`
	KeepDays     int    `json:"keep_days"`
	HasMetadata  bool   `json:"has_metadata"`
}

type StationsResponse struct {
	Stations []StationInfo `json:"stations"`
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
	Playback string `json:"playback"`
	Metadata string `json:"metadata,omitempty"`
}

type RecordingsResponse struct {
	Recordings []Recording `json:"recordings"`
	Station    string      `json:"station"`
}

type MetadataResponse struct {
	Station   string `json:"station"`
	Timestamp string `json:"timestamp"`
	Metadata  string `json:"metadata"`
	FetchedAt string `json:"fetched_at"`
}

type SystemStats struct {
	Uptime          string                 `json:"uptime"`
	TotalRecordings int                    `json:"total_recordings"`
	TotalSize       int64                  `json:"total_size_bytes"`
	StationStats    map[string]StationStat `json:"station_stats"`
	CacheStats      interface{}            `json:"cache_stats"`
}

type StationStat struct {
	Recordings   int       `json:"recordings"`
	SizeBytes    int64     `json:"size_bytes"`
	LastRecorded time.Time `json:"last_recorded"`
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
			Version:   APIVersion,
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
			Version:   APIVersion,
		},
	}

	s.writeJSON(w, status, response)
}

var startTime = time.Now()

// calculateStationStats computes statistics for stationName by scanning its recordings.
func (s *Server) calculateStationStats(stationName string) (totalSize int64, lastSeen time.Time, recordingCount int, err error) {
	recordings, err := s.getRecordings(stationName)
	if err != nil {
		return 0, time.Time{}, 0, err
	}

	// If no recordings, return zero time which will be handled appropriately
	if len(recordings) == 0 {
		return 0, time.Time{}, 0, nil
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

// buildStationInfo creates a StationInfo struct with calculated statistics
func (s *Server) buildStationInfo(stationName string, station config.Station) StationInfo {
	totalSize, lastSeen, recordingCount, _ := s.calculateStationStats(stationName)
	return StationInfo{
		Name:         stationName,
		URL:          station.URL,
		Status:       "active",
		LastRecorded: utils.ToAPIStringOrEmpty(lastSeen, s.config.Timezone),
		Recordings:   recordingCount,
		TotalSize:    totalSize,
		KeepDays:     s.config.GetStationKeepDays(stationName),
		HasMetadata:  station.MetadataURL != "",
	}
}

// buildRecordingURLs creates RecordingURLs struct with proper endpoint URLs
func (s *Server) buildRecordingURLs(stationName, timestamp string, hasMetadata bool) RecordingURLs {
	baseURL := fmt.Sprintf("/api/v1/stations/%s/recordings/%s", stationName, timestamp)
	urls := RecordingURLs{
		Download: baseURL + "/download",
		Playback: baseURL,
	}
	if hasMetadata {
		urls.Metadata = baseURL + "/metadata"
	}
	return urls
}

// hasMetadataForRecording checks if metadata exists for a recording
func (s *Server) hasMetadataForRecording(stationName, timestamp string) bool {
	metadataPath := utils.MetadataPath(s.config.RecordingsDirectory, stationName, timestamp)
	return utils.FileExists(metadataPath)
}

// healthHandler provides basic health check endpoint.
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(startTime).String()

	s.writeJSON(w, http.StatusOK, HealthResponse{
		Status:    "ok",
		Timestamp: time.Now(),
		Version:   version.Version,
		Uptime:    uptime,
	})
}

// readinessHandler checks if the server is ready to serve requests.
func (s *Server) readinessHandler(w http.ResponseWriter, r *http.Request) {
	checks := []Check{
		{Name: "cache", Status: "ok"},
		{Name: "storage", Status: "ok"},
	}

	if _, err := os.Stat(s.config.RecordingsDirectory); os.IsNotExist(err) {
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

// stationDetailsHandler returns detailed information about a specific station.
func (s *Server) stationDetailsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	stationName := vars["station"]

	if !s.validateStationExists(w, stationName) {
		return
	}
	station := s.config.Stations[stationName]

	stationInfo := s.buildStationInfo(stationName, station)

	s.writeAPIResponse(w, http.StatusOK, stationInfo, 1)
}

// recordingHandler returns information about a specific recording.
func (s *Server) recordingHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	stationName := vars["station"]
	timestamp := vars["timestamp"]

	if !s.validateStationExists(w, stationName) {
		return
	}

	recordingPath, exists := s.validateRecordingExists(w, stationName, timestamp)
	if !exists {
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

	hasMetadata := s.hasMetadataForRecording(stationName, timestamp)

	recording := Recording{
		Timestamp:   timestamp,
		StartTime:   utils.ToAPIString(startTime, s.config.Timezone),
		EndTime:     utils.ToAPIString(endTime, s.config.Timezone),
		Duration:    "1h",
		Size:        stat.Size(),
		SizeHuman:   formatFileSize(stat.Size()),
		HasMetadata: hasMetadata,
		URLs:        s.buildRecordingURLs(stationName, timestamp, hasMetadata),
	}

	s.writeAPIResponse(w, http.StatusOK, recording, 1)
}

// downloadRecordingHandler serves a complete recording file for download.
func (s *Server) downloadRecordingHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	stationName := vars["station"]
	timestamp := vars["timestamp"]

	if !s.validateStationExists(w, stationName) {
		return
	}

	recordingPath, exists := s.validateRecordingExists(w, stationName, timestamp)
	if !exists {
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_%s.mp3\"", stationName, timestamp))

	http.ServeFile(w, r, recordingPath)
}

// systemStatsHandler returns comprehensive system statistics.
func (s *Server) systemStatsHandler(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(startTime).String()

	stationStats := make(map[string]StationStat)
	totalRecordings := 0
	totalSize := int64(0)

	for stationName := range s.config.Stations {
		streamSize, lastActive, recordingCount, _ := s.calculateStationStats(stationName)

		stationStats[stationName] = StationStat{
			Recordings:   recordingCount,
			SizeBytes:    streamSize,
			LastRecorded: lastActive,
		}

		totalRecordings += recordingCount
		totalSize += streamSize
	}

	stats := SystemStats{
		Uptime:          uptime,
		TotalRecordings: totalRecordings,
		TotalSize:       totalSize,
		StationStats:    stationStats,
		CacheStats:      s.cache.GetCacheStats(),
	}

	s.writeAPIResponse(w, http.StatusOK, stats, 1)
}

// stationsHandler returns a list of all configured stations with their statistics.
func (s *Server) stationsHandler(w http.ResponseWriter, r *http.Request) {
	stations := make([]StationInfo, 0, len(s.config.Stations))

	for stationName, station := range s.config.Stations {
		stationInfo := s.buildStationInfo(stationName, station)
		stations = append(stations, stationInfo)
	}

	s.writeAPIResponse(w, http.StatusOK, StationsResponse{Stations: stations}, len(stations))
}

// recordingsHandler returns a list of all recordings for a specific station.
func (s *Server) recordingsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	stationName := vars["station"]

	if !s.validateStationExists(w, stationName) {
		return
	}

	rawRecordings, err := s.getRecordings(stationName)
	if err != nil {
		s.logger.Error("failed to list recordings", "error", err)
		s.writeAPIError(w, http.StatusInternalServerError, "Failed to list recordings", err.Error())
		return
	}

	recordings := make([]Recording, len(rawRecordings))
	for i, raw := range rawRecordings {
		startTime, _ := utils.ParseTimestamp(raw.Timestamp, s.config.Timezone)
		endTime := startTime.Add(time.Hour)

		hasMetadata := s.hasMetadataForRecording(stationName, raw.Timestamp)

		recordings[i] = Recording{
			Timestamp:   raw.Timestamp,
			StartTime:   utils.ToAPIString(startTime, s.config.Timezone),
			EndTime:     utils.ToAPIString(endTime, s.config.Timezone),
			Duration:    "1h",
			Size:        raw.Size,
			SizeHuman:   formatFileSize(raw.Size),
			HasMetadata: hasMetadata,
			URLs:        s.buildRecordingURLs(stationName, raw.Timestamp, hasMetadata),
		}
	}

	response := RecordingsResponse{
		Recordings: recordings,
		Station:    stationName,
	}

	s.writeAPIResponse(w, http.StatusOK, response, len(recordings))
}

// metadataHandler returns metadata for a specific recording.
func (s *Server) metadataHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	stationName := vars["station"]
	timestamp := vars["timestamp"]

	if !s.validateStationExists(w, stationName) {
		return
	}

	metadata, err := s.metadata.GetMetadata(utils.StationDirectory(s.config.RecordingsDirectory, stationName), timestamp)
	if err != nil {
		s.writeAPIError(w, http.StatusNotFound, "Metadata not found", err.Error())
		return
	}

	response := MetadataResponse{
		Station:   stationName,
		Timestamp: timestamp,
		Metadata:  metadata,
		FetchedAt: utils.ToAPIString(utils.NowInTimezone(s.config.Timezone), s.config.Timezone),
	}

	s.writeAPIResponse(w, http.StatusOK, response, 1)
}

// formatFileSize converts bytes to human-readable format (KB, MB, GB).
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
