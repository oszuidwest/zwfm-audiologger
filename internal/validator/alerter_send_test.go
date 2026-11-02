package validator

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oszuidwest/zw-graphmail"
	"github.com/oszuidwest/zwfm-audiologger/internal/config"
)

type fakeMailer struct{ sent []*graphmail.Message }

func (f *fakeMailer) Send(_ context.Context, m *graphmail.Message) error {
	f.sent = append(f.sent, m)
	return nil
}

func TestAlerterBuildsHTMLMessages(t *testing.T) {
	fake := &fakeMailer{}
	a := &Alerter{
		config:            &config.AlertConfig{DefaultRecipients: []string{"ops@example.com "}},
		stationRecipients: map[string][]string{"zuidwest": {" team@example.com ", ""}},
		mail:              fake,
	}

	result := &ValidationResult{Station: "zuidwest", Timestamp: "2026-11-02-10", Issues: []string{"<silence>"}}
	if err := a.Send(t.Context(), result); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := a.SendRecordingFailure(t.Context(), "rucphen", "ffmpeg exited", time.Now()); err != nil {
		t.Fatalf("SendRecordingFailure: %v", err)
	}

	if len(fake.sent) != 2 {
		t.Fatalf("sent %d messages, want 2", len(fake.sent))
	}
	tests := []struct {
		msg     *graphmail.Message
		to      []string
		subject string
		body    string
	}{
		{fake.sent[0], []string{"team@example.com"}, "[Audio Logger] Validation failed: zuidwest - 2026-11-02-10", "<li>&lt;silence&gt;</li>"},
		{fake.sent[1], []string{"ops@example.com"}, "[Audio Logger] Recording failed: rucphen", "ffmpeg exited"},
	}
	for _, tt := range tests {
		if !tt.msg.HTML {
			t.Errorf("%q: HTML = false, want true", tt.subject)
		}
		if !slices.Equal(tt.msg.To, tt.to) {
			t.Errorf("%q: To = %q, want %q", tt.subject, tt.msg.To, tt.to)
		}
		if tt.msg.Subject != tt.subject {
			t.Errorf("Subject = %q, want %q", tt.msg.Subject, tt.subject)
		}
		if !strings.Contains(tt.msg.Body, tt.body) {
			t.Errorf("%q: Body = %q, want it to contain %q", tt.subject, tt.msg.Body, tt.body)
		}
	}
}
