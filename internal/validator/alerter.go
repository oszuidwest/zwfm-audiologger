package validator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const (
	graphBaseURL     = "https://graph.microsoft.com/v1.0"
	graphScope       = "https://graph.microsoft.com/.default"
	tokenURLTemplate = "https://login.microsoftonline.com/%s/oauth2/v2.0/token" //nolint:gosec // URL template, not a credential

	emailSubjectPrefix = "[Audio Logger]"
	emailContentType   = "HTML"
)

// guidPattern matches the standard GUID format.
var guidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Alerter sends email alerts via Microsoft Graph API.
type Alerter struct {
	config            *config.AlertConfig
	stationRecipients map[string][]string
	httpClient        *http.Client
	fromAddress       string
}

// NewAlerter creates a new MS Graph email alerter.
func NewAlerter(cfg *config.AlertConfig, stationRecipients map[string][]string) *Alerter {
	if err := validateCredentials(cfg); err != nil {
		slog.Error("invalid graph credentials", "error", err)
		return nil
	}

	// Configure OAuth2 client credentials flow.
	conf := &clientcredentials.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		TokenURL:     fmt.Sprintf(tokenURLTemplate, cfg.TenantID),
		Scopes:       []string{graphScope},
	}

	// Configure base HTTP client with timeout.
	baseClient := &http.Client{Timeout: constants.HTTPClientTimeout}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, baseClient)
	httpClient := conf.Client(ctx)

	return &Alerter{
		config:            cfg,
		stationRecipients: stationRecipients,
		httpClient:        httpClient,
		fromAddress:       cfg.SenderEmail,
	}
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
		return fmt.Errorf("%s must be a valid GUID", fieldName)
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

	message := a.buildMessage(result, recipients)
	return a.sendWithRetry(ctx, message)
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

// graphMailRequest represents an MS Graph sendMail request.
type graphMailRequest struct {
	Message graphMessage `json:"message"`
}

type graphMessage struct {
	Subject      string           `json:"subject"`
	Body         graphBody        `json:"body"`
	ToRecipients []graphRecipient `json:"toRecipients"`
}

type graphBody struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

type graphRecipient struct {
	EmailAddress graphEmailAddress `json:"emailAddress"`
}

type graphEmailAddress struct {
	Address string `json:"address"`
}

// buildMessage creates an MS Graph email message.
func (a *Alerter) buildMessage(result *ValidationResult, recipients []string) *graphMailRequest {
	subject := fmt.Sprintf("%s Validation failed: %s - %s", emailSubjectPrefix, result.Station, result.Timestamp)
	content := buildEmailContent(result)
	toRecipients := buildRecipientList(recipients)

	return &graphMailRequest{
		Message: graphMessage{
			Subject: subject,
			Body: graphBody{
				ContentType: emailContentType,
				Content:     content,
			},
			ToRecipients: toRecipients,
		},
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
			b.WriteString(fmt.Sprintf("<li>%s</li>", issue))
		}
		b.WriteString("</ul>")
	}

	b.WriteString(fmt.Sprintf("<p><small>Validated at: %s</small></p>", result.ValidatedAt.Format(time.RFC3339)))
	b.WriteString("</body></html>")

	return b.String()
}

// writeTableRow writes an HTML table row to the builder.
func writeTableRow(b *strings.Builder, label, value string) {
	fmt.Fprintf(b, "<tr><td><strong>%s:</strong></td><td>%s</td></tr>", label, value)
}

// buildRecipientList converts email addresses to Graph API recipient format.
func buildRecipientList(recipients []string) []graphRecipient {
	result := make([]graphRecipient, 0, len(recipients))
	for _, addr := range recipients {
		addr = strings.TrimSpace(addr)
		if addr != "" {
			result = append(result, graphRecipient{
				EmailAddress: graphEmailAddress{Address: addr},
			})
		}
	}
	return result
}

// sendWithRetry sends an email with automatic retries for transient failures.
func (a *Alerter) sendWithRetry(ctx context.Context, message *graphMailRequest) error {
	apiURL := fmt.Sprintf("%s/users/%s/sendMail", graphBaseURL, url.PathEscape(a.fromAddress))

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error
	retryWait := constants.AlertRetryInitialWait

	for attempt := 0; attempt <= constants.AlertRetryMax; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryWait):
			}
			// Exponential backoff.
			retryWait *= 2
			if retryWait > constants.AlertRetryMaxWait {
				retryWait = constants.AlertRetryMaxWait
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonData))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := a.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to send request: %w", err)
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusAccepted, http.StatusOK, http.StatusNoContent:
			slog.Info("alert email sent", "recipients", len(message.Message.ToRecipients))
			return nil
		case http.StatusTooManyRequests:
			// Parse Retry-After header if present.
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
				if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
					retryWait = time.Duration(seconds) * time.Second
				}
			}
			lastErr = fmt.Errorf("rate limited (429): %s", string(respBody))
			continue
		case http.StatusInternalServerError, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			// Transient server errors - retry.
			lastErr = fmt.Errorf("server error %d: %s", resp.StatusCode, string(respBody))
			continue
		default:
			// Non-retryable error.
			return fmt.Errorf("graph API error %d: %s", resp.StatusCode, string(respBody))
		}
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}
