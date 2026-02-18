package server

import (
	"net/http"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
)

// handleStatus handles requests for recording status.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	// Return basic system status - recordings are scheduled, not tracked in memory.
	status := map[string]any{
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
