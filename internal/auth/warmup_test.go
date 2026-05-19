package auth

import (
	"errors"
	"testing"
)

func TestEnsureConfigNoOpWhenNoAccounts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := EnsureConfig(); err != nil {
		t.Fatalf("EnsureConfig returned error: %v", err)
	}
	cfg, err := LoadConfigOptional()
	if err != nil {
		t.Fatalf("LoadConfigOptional: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected no config when no accounts exist, got %+v", cfg)
	}
}

func TestEnsureConfigPrefersFirstPartyFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	token := &FirstPartyTokenData{
		Flow:     FlowMSALOffice,
		TenantID: "tenant-x",
		Email:    "person@contoso.com",
		Graph:    OAuthTokenBundle{AccessToken: "g", RefreshToken: "r", ExpiresAt: 1},
		Outlook:  OAuthTokenBundle{AccessToken: "o", RefreshToken: "r", ExpiresAt: 1},
	}
	if err := SaveFirstPartyToken("work", token); err != nil {
		t.Fatalf("SaveFirstPartyToken: %v", err)
	}

	if err := EnsureConfig(); err != nil {
		t.Fatalf("EnsureConfig: %v", err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DefaultAuthFlow != FlowMSALOffice {
		t.Fatalf("expected default flow %s, got %s", FlowMSALOffice, cfg.DefaultAuthFlow)
	}
}

func TestEnsureConfigKeepsExistingConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := SaveConfig(&Config{ClientID: "preserved", DefaultAuthFlow: FlowLegacy}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if err := EnsureConfig(); err != nil {
		t.Fatalf("EnsureConfig: %v", err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.ClientID != "preserved" {
		t.Fatalf("expected EnsureConfig to leave existing config alone")
	}
}

func TestFormatWarmupErrors(t *testing.T) {
	if msg := FormatWarmupErrors(nil); msg != "" {
		t.Fatalf("expected empty for nil input, got %q", msg)
	}
	results := []WarmupResult{
		{Alias: "ok", Flow: FlowMSALOffice},
		{Alias: "broken", Flow: FlowMSALOffice, Err: errors.New("boom")},
	}
	msg := FormatWarmupErrors(results)
	if msg == "" {
		t.Fatalf("expected non-empty error summary")
	}
}
