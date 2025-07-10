package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version,omitempty"`
}

// StreamsResponse represents the streams list response
type StreamsResponse struct {
	Streams []string `json:"streams"`
}

// RecordingsResponse represents the recordings list response
type RecordingsResponse struct {
	Recordings []Recording `json:"recordings"`
}

// MetadataResponse represents the metadata response
type MetadataResponse struct {
	Metadata  string    `json:"metadata"`
	Timestamp time.Time `json:"timestamp"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

// Recording represents a single recording
type Recording struct {
	Timestamp string `json:"timestamp"`
	Size      int64  `json:"size"`
	Duration  string `json:"duration,omitempty"`
}

// writeJSON writes a JSON response
func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("Failed to encode JSON response: ", err)
	}
}

// writeError writes an error response
func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, ErrorResponse{
		Error:   http.StatusText(status),
		Code:    status,
		Message: message,
	})
}

// healthHandler returns server health status
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, HealthResponse{
		Status:    "ok",
		Timestamp: time.Now(),
		Version:   "1.0.0",
	})
}

// streamsHandler returns list of available streams
func (s *Server) streamsHandler(w http.ResponseWriter, r *http.Request) {
	// Use Go 1.22+ slices package for better performance
	streams := make([]string, 0, len(s.config.Streams))
	for streamName := range s.config.Streams {
		streams = append(streams, streamName)
	}
	
	s.writeJSON(w, http.StatusOK, StreamsResponse{
		Streams: streams,
	})
}

// recordingsHandler returns list of recordings for a stream
func (s *Server) recordingsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	streamName := vars["stream"]
	
	if _, exists := s.config.Streams[streamName]; !exists {
		s.writeError(w, http.StatusNotFound, "Stream not found")
		return
	}
	
	recordings, err := s.getRecordings(streamName)
	if err != nil {
		s.logger.Error("Failed to list recordings: ", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to list recordings")
		return
	}
	
	s.writeJSON(w, http.StatusOK, RecordingsResponse{
		Recordings: recordings,
	})
}

// metadataHandler returns metadata for a specific recording
func (s *Server) metadataHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	streamName := vars["stream"]
	timestamp := vars["timestamp"]
	
	if _, exists := s.config.Streams[streamName]; !exists {
		s.writeError(w, http.StatusNotFound, "Stream not found")
		return
	}
	
	metadata, err := s.metadata.GetMetadata(s.getStreamDir(streamName), timestamp)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "Metadata not found")
		return
	}
	
	s.writeJSON(w, http.StatusOK, MetadataResponse{
		Metadata:  metadata,
		Timestamp: time.Now(),
	})
}

// fullRecordingHandler serves a complete recording file
func (s *Server) fullRecordingHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	streamName := vars["stream"]
	timestamp := vars["timestamp"]
	
	if _, exists := s.config.Streams[streamName]; !exists {
		s.writeError(w, http.StatusNotFound, "Stream not found")
		return
	}
	
	recordingPath := s.getRecordingPath(streamName, timestamp)
	
	if !s.fileExists(recordingPath) {
		s.writeError(w, http.StatusNotFound, "Recording not found")
		return
	}
	
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_%s.mp3\"", streamName, timestamp))
	
	http.ServeFile(w, r, recordingPath)
}