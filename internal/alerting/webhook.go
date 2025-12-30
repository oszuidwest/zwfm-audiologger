package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const (
	// webhookTimeout is the maximum time to wait for a webhook response.
	webhookTimeout = 10 * time.Second
)

// WebhookConfig holds configuration for webhook alerting.
type WebhookConfig struct {
	URL string `json:"webhook_url"`
}

// WebhookAlerter sends alerts via HTTP POST to a configured webhook URL.
type WebhookAlerter struct {
	config WebhookConfig
	client *http.Client
}

// NewWebhookAlerter creates a new webhook alerter with the given configuration.
// Returns nil if the URL is empty (webhook not configured).
func NewWebhookAlerter(config WebhookConfig) *WebhookAlerter {
	if config.URL == "" {
		return nil
	}

	return &WebhookAlerter{
		config: config,
		client: &http.Client{
			Timeout: webhookTimeout,
		},
	}
}

// Send sends an event to the configured webhook URL.
// This is a fire-and-forget operation: errors are logged but not returned.
func (w *WebhookAlerter) Send(ctx context.Context, event *Event) {
	if w == nil {
		return
	}

	// Marshal event to JSON
	payload, err := json.Marshal(event)
	if err != nil {
		slog.Error("failed to marshal webhook payload", "error", err, "event_type", event.Type)
		return
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.config.URL, bytes.NewReader(payload))
	if err != nil {
		slog.Error("failed to create webhook request", "error", err, "url", w.config.URL)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request (fire-and-forget)
	resp, err := w.client.Do(req)
	if err != nil {
		slog.Error("failed to send webhook", "error", err, "url", w.config.URL, "event_type", event.Type)
		return
	}
	defer func() {
		// Drain body to enable HTTP connection reuse
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		slog.Debug("webhook sent successfully", "url", w.config.URL, "status", resp.StatusCode, "event_type", event.Type)
	} else {
		slog.Warn("webhook returned non-success status", "url", w.config.URL, "status", resp.StatusCode, "event_type", event.Type)
	}
}
