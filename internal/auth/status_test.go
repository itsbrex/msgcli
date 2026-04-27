package auth

import (
	"context"
	"testing"

	"github.com/99designs/keyring"
)

func useEmptyKeyring(t *testing.T) {
	t.Helper()
	arrayRing := keyring.NewArrayKeyring(nil)
	original := keyringOpen
	keyringOpen = func() (keyring.Keyring, error) { return arrayRing, nil }
	t.Cleanup(func() { keyringOpen = original })
}

func TestStatus_NoConfigNoAccounts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	useEmptyKeyring(t)

	report, err := Status(context.Background(), "")
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if report.ConfigExists {
		t.Errorf("ConfigExists = true, want false")
	}
	if report.ClientID != "" {
		t.Errorf("ClientID = %q, want empty", report.ClientID)
	}
	if len(report.Accounts) != 0 {
		t.Errorf("Accounts = %d, want 0", len(report.Accounts))
	}
}

func TestStatus_ConfigPresentTruncatesClientID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	useEmptyKeyring(t)

	const fullID = "11111111-2222-3333-4444-555555555555"
	if err := SaveConfig(&Config{ClientID: fullID, DefaultAuthFlow: FlowLegacy}); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	report, err := Status(context.Background(), "")
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if !report.ConfigExists {
		t.Errorf("ConfigExists = false, want true")
	}
	want := fullID[:8] + "..."
	if report.ClientID != want {
		t.Errorf("ClientID = %q, want %q", report.ClientID, want)
	}
	if report.DefaultFlow != FlowLegacy.String() {
		t.Errorf("DefaultFlow = %q, want %q", report.DefaultFlow, FlowLegacy.String())
	}
}

func TestStatus_ShortClientIDNotTruncated(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	useEmptyKeyring(t)

	if err := SaveConfig(&Config{ClientID: "shortid", DefaultAuthFlow: FlowLegacy}); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	report, err := Status(context.Background(), "")
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if report.ClientID != "shortid" {
		t.Errorf("ClientID = %q, want %q (no truncation for short IDs)", report.ClientID, "shortid")
	}
}

func TestStatus_FiltersToAlias(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	useEmptyKeyring(t)

	if err := SaveToken("alpha", &TokenData{AccessToken: "a", RefreshToken: "ra", ExpiresAt: 9999, Email: "alpha@x.com"}); err != nil {
		t.Fatalf("SaveToken alpha failed: %v", err)
	}
	if err := SaveToken("beta", &TokenData{AccessToken: "b", RefreshToken: "rb", ExpiresAt: 9999, Email: "beta@x.com"}); err != nil {
		t.Fatalf("SaveToken beta failed: %v", err)
	}

	report, err := Status(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if len(report.Accounts) != 1 {
		t.Fatalf("Accounts = %d, want 1", len(report.Accounts))
	}
	if report.Accounts[0].Alias != "alpha" {
		t.Errorf("Alias = %q, want alpha", report.Accounts[0].Alias)
	}
	if report.Accounts[0].Email != "alpha@x.com" {
		t.Errorf("Email = %q, want alpha@x.com", report.Accounts[0].Email)
	}
}
