// Package server provides HTTP endpoints for controlling recordings.
package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/postprocessor"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
)

// handleStatus handles requests for recording status.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	// Return basic system status - recordings are scheduled, not tracked in memory.
	status := map[string]interface{}{
		"message": "System running - recordings scheduled hourly",
		"time":    utils.Now().Format(time.RFC3339),
	}

	writeJSON(w, http.StatusOK, status)
}

// handleHealth handles health check requests.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// handleProgramStart marks when a program starts (commercials end).
func (s *Server) handleProgramStart(w http.ResponseWriter, r *http.Request) {
	station := r.PathValue("station")
	if station == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Station name required"})
		return
	}

	s.postProcessor.MarkProgram(station, postprocessor.MarkStart)
	writeJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Marked program start for %s", station)})
}

// handleProgramStop marks when a program ends (commercials start).
func (s *Server) handleProgramStop(w http.ResponseWriter, r *http.Request) {
	station := r.PathValue("station")
	if station == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Station name required"})
		return
	}

	s.postProcessor.MarkProgram(station, postprocessor.MarkEnd)
	writeJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Marked program end for %s", station)})
}
