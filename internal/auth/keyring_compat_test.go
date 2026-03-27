package auth

import (
	"context"
	"testing"

	"github.com/99designs/keyring"
)

func TestListAccountsIncludesLegacyAndFirstParty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	arrayRing := keyring.NewArrayKeyring(nil)
	originalOpener := keyringOpen
	keyringOpen = func() (keyring.Keyring, error) { return arrayRing, nil }
	t.Cleanup(func() { keyringOpen = originalOpener })

	if err := SaveToken("legacy", &TokenData{AccessToken: "legacy-access", RefreshToken: "legacy-refresh", ExpiresAt: 9999, Email: "legacy@contoso.com"}); err != nil {
		t.Fatalf("SaveToken failed: %v", err)
	}

	if err := SaveFirstPartyToken("firstparty", &FirstPartyTokenData{
		Flow:     FlowMSALOffice,
		TenantID: "tenant-x",
		Email:    "firstparty@contoso.com",
		Graph: OAuthTokenBundle{
			AccessToken:  "graph-access",
			RefreshToken: "graph-refresh",
			ExpiresAt:    9999,
		},
		Outlook: OAuthTokenBundle{
			AccessToken:  "outlook-access",
			RefreshToken: "outlook-refresh",
			ExpiresAt:    9999,
		},
	}); err != nil {
		t.Fatalf("SaveFirstPartyToken failed: %v", err)
	}

	accounts, err := ListAccounts()
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accounts))
	}

	flows := map[string]AuthFlow{}
	for _, acc := range accounts {
		flows[acc.Alias] = acc.Flow
	}
	if flows["legacy"] != FlowLegacy {
		t.Fatalf("expected legacy flow for legacy alias, got %s", flows["legacy"])
	}
	if flows["firstparty"] != FlowMSALOffice {
		t.Fatalf("expected first-party flow for firstparty alias, got %s", flows["firstparty"])
	}
}

func TestGetValidTokenLegacyPathUnchanged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	arrayRing := keyring.NewArrayKeyring(nil)
	originalOpener := keyringOpen
	keyringOpen = func() (keyring.Keyring, error) { return arrayRing, nil }
	t.Cleanup(func() { keyringOpen = originalOpener })

	originalEnvGetter := envGetter
	envGetter = func(key string) string {
		if key == EnvClientID {
			return "client-id"
		}
		return ""
	}
	t.Cleanup(func() { envGetter = originalEnvGetter })

	originalNow := nowUnix
	nowUnix = func() int64 { return 1000 }
	t.Cleanup(func() { nowUnix = originalNow })

	if err := SaveToken("legacy", &TokenData{AccessToken: "legacy-access", RefreshToken: "legacy-refresh", ExpiresAt: 5000, Email: "legacy@contoso.com"}); err != nil {
		t.Fatalf("SaveToken failed: %v", err)
	}

	accessToken, err := GetValidToken(context.Background(), "legacy")
	if err != nil {
		t.Fatalf("GetValidToken failed: %v", err)
	}
	if accessToken != "legacy-access" {
		t.Fatalf("unexpected access token: %s", accessToken)
	}
}
