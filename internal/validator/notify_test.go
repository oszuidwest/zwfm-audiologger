package validator

import (
	"context"
	"testing"
	"time"

	"github.com/oszuidwest/zw-graphmail"
	"github.com/oszuidwest/zwfm-audiologger/internal/config"
)

// hangingMailer blocks until the send context ends, like a Graph API that
// never responds.
type hangingMailer struct{}

func (hangingMailer) Send(ctx context.Context, _ *graphmail.Message) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestNotifyRecordingFailureStopsWhenContextIsCancelled(t *testing.T) {
	m := &Manager{alerter: &Alerter{
		config:            &config.AlertConfig{SenderEmail: "alerts@example.com"},
		stationRecipients: map[string][]string{"station": {"ops@example.com"}},
		mail:              hangingMailer{},
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
		t.Fatal("NotifyRecordingFailure kept waiting after its context was cancelled")
	}
}
