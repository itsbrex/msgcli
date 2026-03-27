package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveTenantIDFromEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/contoso.com/v2.0/.well-known/openid-configuration" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"issuer":"https://login.microsoftonline.com/11111111-2222-3333-4444-555555555555/v2.0"}`))
	}))
	defer server.Close()

	oldBase := identityBaseURL
	oldClient := oauthHTTPClient
	identityBaseURL = server.URL
	oauthHTTPClient = server.Client()
	t.Cleanup(func() {
		identityBaseURL = oldBase
		oauthHTTPClient = oldClient
	})

	tenantID, err := ResolveTenantIDFromEmail(context.Background(), "user@contoso.com")
	if err != nil {
		t.Fatalf("ResolveTenantIDFromEmail returned error: %v", err)
	}
	if tenantID != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("unexpected tenantID: %s", tenantID)
	}
}

func TestExtractTenantIDFromAADURL(t *testing.T) {
	if got := extractTenantIDFromAADURL("https://login.microsoftonline.com/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee/oauth2/v2.0/token"); got != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Fatalf("unexpected tenant from URL: %s", got)
	}
	if got := extractTenantIDFromAADURL("https://login.microsoftonline.com/common/v2.0"); got != "" {
		t.Fatalf("expected empty tenant for common endpoint, got %s", got)
	}
}
