package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var guidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type openIDConfiguration struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

// ResolveTenantIDFromEmail resolves a tenant GUID by querying AAD OpenID config for the email domain.
func ResolveTenantIDFromEmail(ctx context.Context, email string) (string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || !strings.Contains(email, "@") {
		return "", errors.New("valid --email is required to resolve tenant")
	}

	parts := strings.Split(email, "@")
	domain := strings.TrimSpace(parts[len(parts)-1])
	if domain == "" {
		return "", errors.New("unable to parse email domain")
	}

	endpoint := fmt.Sprintf("%s/%s/v2.0/.well-known/openid-configuration", identityBaseURL, url.PathEscape(domain))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("failed to resolve tenant for domain %s: %s", domain, resp.Status)
	}

	var cfg openIDConfiguration
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return "", err
	}

	for _, candidate := range []string{cfg.Issuer, cfg.TokenEndpoint, cfg.AuthorizationEndpoint} {
		if tenant := extractTenantIDFromAADURL(candidate); tenant != "" {
			return tenant, nil
		}
	}

	return "", fmt.Errorf("could not extract tenant ID from OpenID metadata for %s", domain)
}

func extractTenantIDFromAADURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}

	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if guidPattern.MatchString(segment) {
			return strings.ToLower(segment)
		}
	}
	return ""
}
