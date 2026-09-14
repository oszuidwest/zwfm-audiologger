package validator

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNotifyRecordingFailureStopsWhenContextIsCancelled(t *testing.T) {
	m := &Manager{alerter: &Alerter{
		config:            &config.AlertConfig{SenderEmail: "alerts@example.com"},
		stationRecipients: map[string][]string{"station": {"ops@example.com"}},
		// Every attempt hangs until the request context ends, like an
		// unreachable Graph API would.
		httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			<-r.Context().Done()
			return nil, r.Context().Err()
		})},
	}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.NotifyRecordingFailure(ctx, "station", "disk full")
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("NotifyRecordingFailure kept retrying after its context was cancelled")
	}
}
