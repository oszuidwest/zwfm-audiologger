package alerting

import (
	"cmp"
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/smtp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
)

const (
	// emailDialTimeout is the maximum time to establish a connection.
	emailDialTimeout = 10 * time.Second
)

// EmailAlerter sends alerts via SMTP email with StartTLS support.
// It is safe for concurrent use.
type EmailAlerter struct {
	config config.EmailConfig
}

// NewEmailAlerter creates a new email alerter with the given configuration.
// Returns nil if config is nil or required fields (host, from, to) are empty.
func NewEmailAlerter(cfg *config.EmailConfig) *EmailAlerter {
	if cfg == nil || cfg.SMTPHost == "" || cfg.FromAddr == "" || len(cfg.ToAddrs) == 0 {
		return nil
	}

	return &EmailAlerter{
		config: config.EmailConfig{
			SMTPHost: cfg.SMTPHost,
			SMTPPort: cmp.Or(cfg.SMTPPort, 587),
			Username: cfg.Username,
			Password: cfg.Password,
			FromAddr: cfg.FromAddr,
			ToAddrs:  slices.Clone(cfg.ToAddrs),
		},
	}
}

// Send sends an event via email.
// This is a fire-and-forget operation: errors are logged but not returned.
func (e *EmailAlerter) Send(ctx context.Context, event *Event) {
	if e == nil {
		return
	}

	// Build email message
	subject := e.buildSubject(event)
	body := e.buildBody(event)

	// Send email with context awareness
	if err := e.sendMail(ctx, e.config.ToAddrs, subject, body); err != nil {
		slog.Error("failed to send email alert", "error", err, "event_type", event.Type, "recipients", e.config.ToAddrs)
		return
	}

	slog.Debug("email alert sent successfully", "event_type", event.Type, "recipients", e.config.ToAddrs)
}

// sendMail sends an email using SMTP with StartTLS support.
func (e *EmailAlerter) sendMail(ctx context.Context, recipients []string, subject, body string) error {
	addr := net.JoinHostPort(e.config.SMTPHost, strconv.Itoa(e.config.SMTPPort))

	// Create dialer with timeout
	dialer := &net.Dialer{Timeout: emailDialTimeout}

	// Establish TCP connection
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to dial SMTP server: %w", err)
	}

	// Create SMTP client
	client, err := smtp.NewClient(conn, e.config.SMTPHost)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer func() { _ = client.Close() }()

	// Try StartTLS if available
	hasCredentials := e.config.Username != "" && e.config.Password != ""
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{
			ServerName: e.config.SMTPHost,
			MinVersion: tls.VersionTLS12,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			// If credentials are configured, refuse to send them over plaintext
			if hasCredentials {
				return fmt.Errorf("StartTLS failed and credentials configured (refusing plaintext auth): %w", err)
			}
			slog.Warn("StartTLS failed, continuing without encryption", "error", err)
		}
	} else if hasCredentials {
		// Server doesn't support STARTTLS but we have credentials
		return fmt.Errorf("SMTP server does not support STARTTLS, refusing to send credentials in plaintext")
	}

	// Authenticate if credentials are provided
	if hasCredentials {
		auth := smtp.PlainAuth("", e.config.Username, e.config.Password, e.config.SMTPHost)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}

	// Set sender
	if err := client.Mail(e.config.FromAddr); err != nil {
		return fmt.Errorf("MAIL command failed: %w", err)
	}

	// Set recipients
	for _, rcpt := range recipients {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("RCPT command failed for %s: %w", rcpt, err)
		}
	}

	// Send message body
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA command failed: %w", err)
	}

	msg := e.formatMessage(recipients, subject, body)
	if _, err := wc.Write([]byte(msg)); err != nil {
		_ = wc.Close()
		return fmt.Errorf("failed to write message: %w", err)
	}

	if err := wc.Close(); err != nil {
		return fmt.Errorf("failed to close message writer: %w", err)
	}

	return client.Quit()
}

// buildSubject creates the email subject line.
func (e *EmailAlerter) buildSubject(event *Event) string {
	prefix := "[AudioLogger]"
	typeStr := string(event.Type)
	if event.Station != "" {
		return fmt.Sprintf("%s %s - %s", prefix, typeStr, event.Station)
	}
	return fmt.Sprintf("%s %s", prefix, typeStr)
}

// buildBody creates the plain text email body.
func (e *EmailAlerter) buildBody(event *Event) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Event: %s\n", event.Type))
	sb.WriteString(fmt.Sprintf("Time: %s\n", event.Timestamp.Format(time.RFC3339)))

	if event.Station != "" {
		sb.WriteString(fmt.Sprintf("Station: %s\n", event.Station))
	}

	sb.WriteString(fmt.Sprintf("\nMessage: %s\n", event.Message))

	if len(event.Details) > 0 {
		sb.WriteString("\nDetails:\n")
		for key, value := range event.Details {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", key, value))
		}
	}

	return sb.String()
}

// formatMessage creates a properly formatted email message with headers.
// Includes RFC 5322 required headers (From, To, Date) and recommended headers (Message-ID).
func (e *EmailAlerter) formatMessage(recipients []string, subject, body string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("From: %s\r\n", e.config.FromAddr))
	sb.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(recipients, ", ")))
	sb.WriteString(fmt.Sprintf("Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject)))
	sb.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	sb.WriteString(fmt.Sprintf("Message-ID: <%d@audiologger>\r\n", time.Now().UnixNano()))
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(body)

	return sb.String()
}
