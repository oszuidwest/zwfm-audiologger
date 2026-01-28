package server

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// authenticate provides simple authentication middleware.
func (s *Server) authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		station := r.PathValue("station")

		if station == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Station name required"})
			return
		}

		// Check if station exists in config.
		stationConfig, exists := s.config.Stations[station]
		if !exists {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Unknown station"})
			return
		}

		// Simple API key check.
		expectedSecret := stationConfig.APISecret
		if expectedSecret == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "No API secret configured"})
			return
		}

		// Check X-API-Key header (most common pattern).
		if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
			if subtle.ConstantTimeCompare([]byte(apiKey), []byte(expectedSecret)) == 1 {
				next(w, r)
				return
			}
		}

		// Check Authorization header with Bearer token.
		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			if token, found := strings.CutPrefix(authHeader, "Bearer "); found {
				if subtle.ConstantTimeCompare([]byte(token), []byte(expectedSecret)) == 1 {
					next(w, r)
					return
				}
			}
		}

		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API key"})
	}
}
