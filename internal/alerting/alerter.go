package alerting

import (
	"context"
	"log/slog"
	"sync"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
)

// Alerter defines the interface for sending alerts.
type Alerter interface {
	// Send sends an event to this alerter.
	// Implementations should be fire-and-forget: log errors but don't return them.
	Send(ctx context.Context, event *Event)
}

// Config holds the combined configuration for all alerters.
type Config struct {
	WebhookURL string
	Email      *config.EmailConfig
}

// Manager coordinates sending alerts to all configured alerters.
// It tracks station state to implement rate limiting (first failure + recovery).
type Manager struct {
	alerters []Alerter
	states   map[string]*stationState
	mu       sync.Mutex
}

// NewManager creates a new alert manager with the provided configuration.
// Alerters that are not configured (nil or empty required fields) are skipped.
func NewManager(cfg *Config) *Manager {
	m := &Manager{
		alerters: make([]Alerter, 0, 2),
		states:   make(map[string]*stationState),
	}

	if cfg == nil {
		slog.Info("No alerters configured")
		return m
	}

	// Add webhook alerter if configured
	if cfg.WebhookURL != "" {
		webhook := NewWebhookAlerter(WebhookConfig{URL: cfg.WebhookURL})
		if webhook != nil {
			m.alerters = append(m.alerters, webhook)
			slog.Info("Webhook alerter configured", "url", cfg.WebhookURL)
		}
	}

	// Add email alerter if configured
	if cfg.Email != nil {
		if email := NewEmailAlerter(cfg.Email); email != nil {
			m.alerters = append(m.alerters, email)
			slog.Info("Email alerter configured", "recipients", cfg.Email.ToAddrs)
		}
	}

	if len(m.alerters) == 0 {
		slog.Info("No alerters configured")
	}

	return m
}

// Alert sends an event to all configured alerters with rate limiting.
// For station-specific events, it tracks state to:
// - Only alert on first failure (not repeatedly while station is down)
// - Alert on recovery when station goes from failed to working
//
// This method is safe for concurrent use.
func (m *Manager) Alert(ctx context.Context, event *Event) {
	if len(m.alerters) == 0 {
		return
	}

	// Atomically check and update state to prevent race conditions
	if !m.checkAndUpdateState(event) {
		slog.Debug("alert suppressed due to rate limiting",
			"event_type", event.Type,
			"station", event.Station)
		return
	}

	// Send to all alerters asynchronously
	for _, alerter := range m.alerters {
		go alerter.Send(ctx, event)
	}

	slog.Info("Alert sent",
		"event_type", event.Type,
		"station", event.Station,
		"message", event.Message,
		"alerters", len(m.alerters))
}

// checkAndUpdateState atomically checks if an alert should be sent and updates state.
// Returns true if the alert should be sent, false if it should be suppressed.
// This combines the check and update to prevent race conditions.
func (m *Manager) checkAndUpdateState(event *Event) bool {
	// Non-station events always go through, no state to track
	if event.Station == "" {
		return true
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.states[event.Station]

	switch event.Type {
	case EventStreamOffline, EventRecordingIncomplete:
		// Only alert on first failure
		if !exists || !state.failed {
			m.states[event.Station] = &stationState{
				failed:    true,
				lastEvent: event.Timestamp,
			}
			return true
		}
		// Already in failed state, suppress
		return false

	case EventStreamRecovered:
		// Only alert if we were previously in failed state
		if exists && state.failed {
			m.states[event.Station] = &stationState{
				failed:    false,
				lastEvent: event.Timestamp,
			}
			return true
		}
		// Wasn't tracked as failed, suppress recovery alert
		return false

	default:
		// Other events (like disk space) always go through
		return true
	}
}
