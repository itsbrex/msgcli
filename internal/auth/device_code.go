package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// Microsoft identity platform endpoints
	legacyAuthorizeEndpoint = "https://login.microsoftonline.com/common/oauth2/v2.0/devicecode"
	legacyTokenEndpoint     = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	graphEndpoint           = "https://graph.microsoft.com/v1.0"

	// Scopes
	legacyScopes          = "offline_access User.Read Mail.ReadWrite Mail.Send Calendars.ReadWrite"
	firstPartyGraphScopes = "https://graph.microsoft.com/.default offline_access"
	outlookScopes         = "https://outlook.office365.com/.default offline_access"

	legacyRefreshSkewSeconds     int64 = 300
	firstPartyRefreshSkewSeconds int64 = 60
)

var (
	oauthHTTPClient = http.DefaultClient
	nowUnix         = func() int64 { return time.Now().Unix() }
	identityBaseURL = "https://login.microsoftonline.com"
)

// DeviceCodeResponse is the initial response from the device code endpoint
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	Message         string `json:"message"`
}

// TokenResponse is the response from the token endpoint
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	IDToken      string `json:"id_token,omitempty"`
	Error        string `json:"error,omitempty"`
	ErrorDesc    string `json:"error_description,omitempty"`
}

// UserInfo holds basic user information from Graph API
type UserInfo struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	UserPrincipalName string `json:"userPrincipalName"`
	Mail              string `json:"mail"`
}

// StartDeviceCodeFlow initiates the legacy device code authentication flow.
func StartDeviceCodeFlow(ctx context.Context, clientID string) (*DeviceCodeResponse, error) {
	return startDeviceCodeFlow(ctx, legacyAuthorizeEndpoint, clientID, legacyScopes)
}

// StartFirstPartyDeviceCodeFlow initiates first-party device auth for a tenant.
func StartFirstPartyDeviceCodeFlow(ctx context.Context, tenantID string) (*DeviceCodeResponse, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.New("tenant_id is required for msal-office flow")
	}
	return startDeviceCodeFlow(ctx, tenantDeviceCodeEndpoint(tenantID), FirstPartyClientID, firstPartyGraphScopes)
}

// PollForToken polls the legacy token endpoint until authentication completes.
func PollForToken(ctx context.Context, clientID string, deviceCode string, interval int) (*TokenResponse, error) {
	return pollForToken(ctx, legacyTokenEndpoint, clientID, deviceCode, interval)
}

// PollForFirstPartyGraphToken polls the tenant token endpoint for first-party Graph tokens.
func PollForFirstPartyGraphToken(ctx context.Context, tenantID, deviceCode string, interval int) (*TokenResponse, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.New("tenant_id is required for first-party token polling")
	}
	return pollForToken(ctx, tenantTokenEndpoint(tenantID), FirstPartyClientID, deviceCode, interval)
}

// RefreshAccessToken uses a refresh token to get a new legacy Graph token.
func RefreshAccessToken(ctx context.Context, clientID, refreshToken string) (*TokenResponse, error) {
	return exchangeRefreshToken(ctx, legacyTokenEndpoint, clientID, refreshToken, legacyScopes)
}

// GetValidToken returns a valid Graph access token for the given account alias.
func GetValidToken(ctx context.Context, alias string) (string, error) {
	flow, err := DetectAccountFlow(alias)
	if err != nil {
		return "", err
	}

	switch flow {
	case FlowLegacy:
		return getValidLegacyToken(ctx, alias, false)
	case FlowMSALOffice:
		return getValidFirstPartyGraphToken(ctx, alias, false)
	default:
		return "", fmt.Errorf("unsupported auth flow: %s", flow)
	}
}

// GetValidOutlookToken returns a valid Outlook token for msal-office accounts.
func GetValidOutlookToken(ctx context.Context, alias string) (string, error) {
	flow, err := DetectAccountFlow(alias)
	if err != nil {
		return "", err
	}
	if flow != FlowMSALOffice {
		return "", fmt.Errorf("outlook token is only available for %s accounts", FlowMSALOffice)
	}
	token, err := ensureFirstPartyTokenFreshness(ctx, alias, false)
	if err != nil {
		return "", err
	}
	return token.Outlook.AccessToken, nil
}

// RefreshAccountTokens forces a refresh for both legacy and first-party accounts.
func RefreshAccountTokens(ctx context.Context, alias string, flow AuthFlow) error {
	switch flow {
	case FlowLegacy:
		_, err := getValidLegacyToken(ctx, alias, true)
		if err != nil {
			return fmt.Errorf("refresh legacy token: %w", err)
		}
		return nil
	case FlowMSALOffice:
		_, err := getValidFirstPartyGraphToken(ctx, alias, true)
		if err != nil {
			return fmt.Errorf("refresh first-party token: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported auth flow: %s", flow)
	}
}

// DetectAccountFlow returns the persisted flow for an alias.
func DetectAccountFlow(alias string) (AuthFlow, error) {
	if FirstPartyTokenExists(alias) {
		return FlowMSALOffice, nil
	}
	if _, err := loadLegacyToken(alias); err == nil {
		return FlowLegacy, nil
	}
	return "", fmt.Errorf("%w: account '%s' not found", ErrAccountNotFound, alias)
}

// ResolveAccount returns the account alias to use, resolving default if needed.
func ResolveAccount(accountFlag string) (string, error) {
	if accountFlag != "" {
		return accountFlag, nil
	}
	return GetDefaultAccount()
}
