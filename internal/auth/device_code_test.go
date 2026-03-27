package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExchangeRefreshToken(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		if got := r.Form.Get("client_id"); got != "client-id" {
			t.Fatalf("unexpected client_id: %s", got)
		}
		if got := r.Form.Get("refresh_token"); got != "refresh-token" {
			t.Fatalf("unexpected refresh_token: %s", got)
		}
		if got := r.Form.Get("scope"); got != "scope-x" {
			t.Fatalf("unexpected scope: %s", got)
		}

		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
			ExpiresIn:    3600,
		})
	}))
	defer server.Close()

	oldClient := oauthHTTPClient
	oauthHTTPClient = server.Client()
	t.Cleanup(func() { oauthHTTPClient = oldClient })

	resp, err := exchangeRefreshToken(context.Background(), server.URL, "client-id", "refresh-token", "scope-x")
	if err != nil {
		t.Fatalf("exchangeRefreshToken returned error: %v", err)
	}
	if resp.AccessToken != "new-access" || resp.RefreshToken != "new-refresh" {
		t.Fatalf("unexpected token response: %+v", resp)
	}
}

func TestRefreshFirstPartyTokenBundles(t *testing.T) {
	var scopes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		scope := r.Form.Get("scope")
		scopes = append(scopes, scope)

		switch scope {
		case firstPartyGraphScopes:
			_ = json.NewEncoder(w).Encode(TokenResponse{AccessToken: "graph-new", RefreshToken: "refresh-new", ExpiresIn: 3600})
		case outlookScopes:
			_ = json.NewEncoder(w).Encode(TokenResponse{AccessToken: "outlook-new", RefreshToken: "outlook-refresh", ExpiresIn: 1800})
		default:
			t.Fatalf("unexpected scope %q", scope)
		}
	}))
	defer server.Close()

	oldClient := oauthHTTPClient
	oldBaseURL := identityBaseURL
	oldNow := nowUnix
	oauthHTTPClient = server.Client()
	identityBaseURL = server.URL
	nowUnix = func() int64 { return 1000 }
	t.Cleanup(func() {
		oauthHTTPClient = oldClient
		identityBaseURL = oldBaseURL
		nowUnix = oldNow
	})

	token := &FirstPartyTokenData{
		Flow:     FlowMSALOffice,
		TenantID: "tenant-x",
		Email:    "person@contoso.com",
		Graph: OAuthTokenBundle{
			RefreshToken: "old-refresh",
		},
	}

	if err := refreshFirstPartyTokenBundles(context.Background(), token); err != nil {
		t.Fatalf("refreshFirstPartyTokenBundles returned error: %v", err)
	}
	if token.Graph.AccessToken != "graph-new" {
		t.Fatalf("expected graph token update, got %+v", token.Graph)
	}
	if token.Outlook.AccessToken != "outlook-new" {
		t.Fatalf("expected outlook token update, got %+v", token.Outlook)
	}
	if token.Graph.ExpiresAt != 4600 {
		t.Fatalf("unexpected graph expiry %d", token.Graph.ExpiresAt)
	}
	if token.Outlook.ExpiresAt != 2800 {
		t.Fatalf("unexpected outlook expiry %d", token.Outlook.ExpiresAt)
	}
	if len(scopes) != 2 || scopes[0] != firstPartyGraphScopes || scopes[1] != outlookScopes {
		t.Fatalf("unexpected refresh scope order: %#v", scopes)
	}
}

func TestShouldRefreshTokenThreshold(t *testing.T) {
	oldNow := nowUnix
	nowUnix = func() int64 { return 1000 }
	t.Cleanup(func() { nowUnix = oldNow })

	if !shouldRefreshToken(1050, 60) {
		t.Fatalf("expected token expiring within threshold to refresh")
	}
	if shouldRefreshToken(1061, 60) {
		t.Fatalf("expected token outside threshold to avoid refresh")
	}
}
