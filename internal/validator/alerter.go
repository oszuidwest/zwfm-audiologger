package validator

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/oszuidwest/zw-graphmail"
	"github.com/oszuidwest/zwfm-audiologger/internal/config"
)

const emailSubjectPrefix = "[Audio Logger]"

// guidPattern matches the standard GUID format.
var guidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// mailer sends one message. *graphmail.Client implements it.
type mailer interface {
	Send(ctx context.Context, m *graphmail.Message) error
}

// Alerter sends email alerts via Microsoft Graph API.
type Alerter struct {
	config            *config.AlertConfig
	stationRecipients map[string][]string
	mail              mailer
}

// NewAlerter creates a new MS Graph email alerter. It returns an error when the
// Graph credentials are missing or malformed, so that a misconfigured alerter
// fails at startup instead of being silently disabled.
func NewAlerter(cfg *config.AlertConfig, stationRecipients map[string][]string) (*Alerter, error) {
	if err := validateCredentials(cfg); err != nil {
		return nil, err
	}

	client, err := graphmail.New(graphmail.Config{
		TenantID:     cfg.TenantID,
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		From:         cfg.SenderEmail,
	})
	if err != nil {
		return nil, err
	}

	return &Alerter{
		config:            cfg,
		stationRecipients: stationRecipients,
		mail:              client,
	}, nil
}

// validateCredentials checks that required credential fields are present and valid.
func validateCredentials(cfg *config.AlertConfig) error {
	if err := validateGUIDField(cfg.TenantID, "tenant ID"); err != nil {
		return err
	}
	if err := validateGUIDField(cfg.ClientID, "client ID"); err != nil {
		return err
	}
	if cfg.ClientSecret == "" {
		return fmt.Errorf("client secret is required")
	}
	if cfg.SenderEmail == "" {
		return fmt.Errorf("sender email is required")
	}
	return nil
}

// validateGUIDField validates that a field contains a valid GUID.
func validateGUIDField(value, fieldName string) error {
	if value == "" {
		return fmt.Errorf("%s is required", fieldName)
	}
	if !guidPattern.MatchString(value) {
		return fmt.Errorf("%s must be a valid guid", fieldName)
	}
	return nil
}

// Send sends an alert email for an invalid validation result.
func (a *Alerter) Send(ctx context.Context, result *ValidationResult) error {
	recipients := a.getRecipients(result.Station)
	if len(recipients) == 0 {
		slog.Warn("no alert recipients configured", "station", result.Station)
		return nil
	}

	return a.send(ctx, a.buildMessage(result, recipients))
}

// SendRecordingFailure sends an alert email when a recording fails to be created.
func (a *Alerter) SendRecordingFailure(ctx context.Context, station, reason string, failedAt time.Time) error {
	recipients := a.getRecipients(station)
	if len(recipients) == 0 {
		slog.Warn("no alert recipients configured", "station", station)
		return nil
	}

	subject := fmt.Sprintf("%s Recording failed: %s", emailSubjectPrefix, station)
	content := buildRecordingFailureContent(station, reason, failedAt)

	return a.send(ctx, &graphmail.Message{
		To:      buildRecipientList(recipients),
		Subject: subject,
		Body:    content,
		HTML:    true,
	})
}

// buildRecordingFailureContent constructs an HTML email body for a recording failure.
func buildRecordingFailureContent(station, reason string, failedAt time.Time) string {
	var b strings.Builder

	b.WriteString("<html><body>")
	b.WriteString("<h2>Recording Failed</h2>")
	b.WriteString("<table style='border-collapse: collapse;'>")
	writeTableRow(&b, "Station", station)
	writeTableRow(&b, "Failed at", failedAt.Format(time.RFC3339))
	writeTableRow(&b, "Reason", reason)
	b.WriteString("</table>")
	b.WriteString("</body></html>")

	return b.String()
}

// getRecipients returns the email recipients for a station.
func (a *Alerter) getRecipients(station string) []string {
	// Check station-specific recipients first.
	if recipients, ok := a.stationRecipients[station]; ok && len(recipients) > 0 {
		return recipients
	}
	// Fall back to default recipients.
	return a.config.DefaultRecipients
}

// buildMessage creates an MS Graph email message.
func (a *Alerter) buildMessage(result *ValidationResult, recipients []string) *graphmail.Message {
	return &graphmail.Message{
		To:      buildRecipientList(recipients),
		Subject: fmt.Sprintf("%s Validation failed: %s - %s", emailSubjectPrefix, result.Station, result.Timestamp),
		Body:    buildEmailContent(result),
		HTML:    true,
	}
}

// buildEmailContent constructs the HTML email body.
func buildEmailContent(result *ValidationResult) string {
	var b strings.Builder

	b.WriteString("<html><body>")
	b.WriteString("<h2>Recording Validation Failed</h2>")
	b.WriteString("<table style='border-collapse: collapse;'>")

	writeTableRow(&b, "Station", result.Station)
	writeTableRow(&b, "Timestamp", result.Timestamp)
	writeTableRow(&b, "Duration", fmt.Sprintf("%.1f seconds", result.DurationSecs))
	writeTableRow(&b, "Silence", fmt.Sprintf("%.1f%%", result.SilencePercent))
	writeTableRow(&b, "Loop", fmt.Sprintf("%.1f%%", result.LoopPercent))

	b.WriteString("</table>")

	if len(result.Issues) > 0 {
		b.WriteString("<h3>Issues:</h3><ul>")
		for _, issue := range result.Issues {
			fmt.Fprintf(&b, "<li>%s</li>", html.EscapeString(issue))
		}
		b.WriteString("</ul>")
	}

	fmt.Fprintf(&b, "<p><small>Validated at: %s</small></p>", result.ValidatedAt.Format(time.RFC3339))
	b.WriteString("</body></html>")

	return b.String()
}

// writeTableRow writes an HTML table row to the builder, escaping the label and value.
func writeTableRow(b *strings.Builder, label, value string) {
	fmt.Fprintf(b, "<tr><td><strong>%s:</strong></td><td>%s</td></tr>", html.EscapeString(label), html.EscapeString(value))
}

// buildRecipientList trims the addresses and drops empty ones.
func buildRecipientList(recipients []string) []string {
	result := make([]string, 0, len(recipients))
	for _, addr := range recipients {
		addr = strings.TrimSpace(addr)
		if addr != "" {
			result = append(result, addr)
		}
	}
	return result
}

// send delivers the message. The client retries transient Graph failures.
func (a *Alerter) send(ctx context.Context, message *graphmail.Message) error {
	if err := a.mail.Send(ctx, message); err != nil {
		return err
	}
	slog.Info("alert email sent", "recipients", len(message.To))
	return nil
}
