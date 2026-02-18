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
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const (
	graphBaseURL     = "https://graph.microsoft.com/v1.0"
	graphScope       = "https://graph.microsoft.com/.default"
	tokenURLTemplate = "https://login.microsoftonline.com/%s/oauth2/v2.0/token" //nolint:gosec // URL template, not a credential

	// Retry settings.
	maxRetries       = 3
	initialRetryWait = 1 * time.Second
	maxRetryWait     = 30 * time.Second

	// HTTP client timeout.
	httpTimeout = 30 * time.Second
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
		slog.Error("Invalid Graph credentials", "error", err)
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
	baseClient := &http.Client{Timeout: httpTimeout}
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
	if cfg.TenantID == "" {
		return fmt.Errorf("tenant ID is required")
	}
	if !guidPattern.MatchString(cfg.TenantID) {
		return fmt.Errorf("tenant ID must be a valid GUID")
	}
	if cfg.ClientID == "" {
		return fmt.Errorf("client ID is required")
	}
	if !guidPattern.MatchString(cfg.ClientID) {
		return fmt.Errorf("client ID must be a valid GUID")
	}
	if cfg.ClientSecret == "" {
		return fmt.Errorf("client secret is required")
	}
	if cfg.SenderEmail == "" {
		return fmt.Errorf("sender email is required")
	}
	return nil
}

// Send sends an alert email for an invalid validation result.
func (a *Alerter) Send(ctx context.Context, result *ValidationResult) error {
	recipients := a.getRecipients(result.Station)
	if len(recipients) == 0 {
		slog.Warn("No alert recipients configured", "station", result.Station)
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
	subject := fmt.Sprintf("[Audio Logger] Validation failed: %s - %s", result.Station, result.Timestamp)

	var contentBuilder strings.Builder
	contentBuilder.WriteString("<html><body>")
	contentBuilder.WriteString("<h2>Recording Validation Failed</h2>")
	contentBuilder.WriteString("<table style='border-collapse: collapse;'>")
	contentBuilder.WriteString(fmt.Sprintf("<tr><td><strong>Station:</strong></td><td>%s</td></tr>", result.Station))
	contentBuilder.WriteString(fmt.Sprintf("<tr><td><strong>Timestamp:</strong></td><td>%s</td></tr>", result.Timestamp))
	contentBuilder.WriteString(fmt.Sprintf("<tr><td><strong>Duration:</strong></td><td>%.1f seconds</td></tr>", result.DurationSecs))
	contentBuilder.WriteString(fmt.Sprintf("<tr><td><strong>Silence:</strong></td><td>%.1f%%</td></tr>", result.SilencePercent))
	contentBuilder.WriteString(fmt.Sprintf("<tr><td><strong>Loop:</strong></td><td>%.1f%%</td></tr>", result.LoopPercent))
	contentBuilder.WriteString("</table>")

	if len(result.Issues) > 0 {
		contentBuilder.WriteString("<h3>Issues:</h3><ul>")
		for _, issue := range result.Issues {
			contentBuilder.WriteString(fmt.Sprintf("<li>%s</li>", issue))
		}
		contentBuilder.WriteString("</ul>")
	}

	contentBuilder.WriteString(fmt.Sprintf("<p><small>Validated at: %s</small></p>", result.ValidatedAt.Format(time.RFC3339)))
	contentBuilder.WriteString("</body></html>")

	toRecipients := make([]graphRecipient, 0, len(recipients))
	for _, addr := range recipients {
		addr = strings.TrimSpace(addr)
		if addr != "" {
			toRecipients = append(toRecipients, graphRecipient{
				EmailAddress: graphEmailAddress{Address: addr},
			})
		}
	}

	return &graphMailRequest{
		Message: graphMessage{
			Subject: subject,
			Body: graphBody{
				ContentType: "HTML",
				Content:     contentBuilder.String(),
			},
			ToRecipients: toRecipients,
		},
	}
}

// sendWithRetry sends an email with automatic retries for transient failures.
func (a *Alerter) sendWithRetry(ctx context.Context, message *graphMailRequest) error {
	apiURL := fmt.Sprintf("%s/users/%s/sendMail", graphBaseURL, url.PathEscape(a.fromAddress))

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	retryWait := initialRetryWait

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryWait):
			}
			// Exponential backoff.
			retryWait *= 2
			if retryWait > maxRetryWait {
				retryWait = maxRetryWait
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonData))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := a.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("send request: %w", err)
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusAccepted, http.StatusOK, http.StatusNoContent:
			slog.Info("Alert email sent", "recipients", len(message.Message.ToRecipients))
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
