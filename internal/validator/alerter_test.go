package validator_test

import (
	"strings"
	"testing"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/validator"
)

const validGUID = "11111111-2222-3333-4444-555555555555"

func validAlertConfig() *config.AlertConfig {
	return &config.AlertConfig{
		Enabled:      true,
		TenantID:     validGUID,
		ClientID:     validGUID,
		ClientSecret: "secret",
		SenderEmail:  "logger@example.com",
	}
}

func TestNewAlerterValidatesCredentials(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(cfg *config.AlertConfig)
		wantErr string
	}{
		{name: "valid", mutate: func(*config.AlertConfig) {}, wantErr: ""},
		{name: "missing tenant id", mutate: func(c *config.AlertConfig) { c.TenantID = "" }, wantErr: "tenant ID is required"},
		{name: "malformed tenant id", mutate: func(c *config.AlertConfig) { c.TenantID = "not-a-guid" }, wantErr: "tenant ID must be a valid guid"},
		{name: "missing client id", mutate: func(c *config.AlertConfig) { c.ClientID = "" }, wantErr: "client ID is required"},
		{name: "malformed client id", mutate: func(c *config.AlertConfig) { c.ClientID = "1234" }, wantErr: "client ID must be a valid guid"},
		{name: "missing client secret", mutate: func(c *config.AlertConfig) { c.ClientSecret = "" }, wantErr: "client secret is required"},
		{name: "missing sender email", mutate: func(c *config.AlertConfig) { c.SenderEmail = "" }, wantErr: "sender email is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validAlertConfig()
			tt.mutate(cfg)

			alerter, err := validator.NewAlerter(cfg, nil)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("NewAlerter returned error: %v", err)
				}
				if alerter == nil {
					t.Fatal("NewAlerter returned nil alerter")
				}
				return
			}
			if err == nil {
				t.Fatal("NewAlerter returned nil error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("NewAlerter error = %v, want substring %q", err, tt.wantErr)
			}
			if alerter != nil {
				t.Fatal("NewAlerter returned non-nil alerter alongside error")
			}
		})
	}
}

func TestNewFailsFastOnInvalidAlertConfig(t *testing.T) {
	cfg := &config.Config{
		RecordingsDir: t.TempDir(),
		Validation: &config.ValidationConfig{
			Enabled: true,
			Alert: &config.AlertConfig{
				Enabled:  true,
				TenantID: "not-a-guid",
			},
		},
	}

	if _, err := validator.New(cfg); err == nil {
		t.Fatal("validator.New returned nil error for invalid alert config")
	}
}
