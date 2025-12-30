// Package alerting provides alerting capabilities for stream monitoring events.
package alerting

import "time"

// EventType represents the type of alerting event.
type EventType string

const (
	// EventStreamOffline indicates a station stream has gone offline.
	EventStreamOffline EventType = "stream_offline"
	// EventStreamRecovered indicates a station stream has recovered after being offline.
	EventStreamRecovered EventType = "stream_recovered"
	// EventRecordingIncomplete indicates a recording did not complete successfully.
	EventRecordingIncomplete EventType = "recording_incomplete"
	// EventDiskSpaceLow indicates disk space is running low.
	EventDiskSpaceLow EventType = "disk_space_low"
)

// Event represents an alerting event that should be sent to configured alerters.
type Event struct {
	Type      EventType         `json:"type"`
	Station   string            `json:"station,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Message   string            `json:"message"`
	Details   map[string]string `json:"details,omitempty"`
}

// NewEvent creates a new Event with the current timestamp.
func NewEvent(eventType EventType, station, message string) *Event {
	return &Event{
		Type:      eventType,
		Station:   station,
		Timestamp: time.Now(),
		Message:   message,
		Details:   make(map[string]string),
	}
}

// WithDetail adds a detail to the event and returns the event for chaining.
func (e *Event) WithDetail(key, value string) *Event {
	if e.Details == nil {
		e.Details = make(map[string]string)
	}
	e.Details[key] = value
	return e
}

// stationState tracks the current state of a station for rate limiting alerts.
type stationState struct {
	failed    bool
	lastEvent time.Time
}
