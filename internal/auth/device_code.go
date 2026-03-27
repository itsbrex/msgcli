package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
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

func startDeviceCodeFlow(ctx context.Context, endpoint, clientID, scope string) (*DeviceCodeResponse, error) {
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("scope", scope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var dcr DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&dcr); err != nil {
		return nil, err
	}
	if dcr.DeviceCode == "" {
		return nil, errors.New("device code endpoint returned an invalid response")
	}
	return &dcr, nil
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

func pollForToken(ctx context.Context, endpoint, clientID, deviceCode string, interval int) (*TokenResponse, error) {
	if interval < 1 {
		interval = 5
	}

	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	data.Set("device_code", deviceCode)

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			resp, err := oauthHTTPClient.Do(req)
			if err != nil {
				return nil, err
			}

			var tr TokenResponse
			err = json.NewDecoder(resp.Body).Decode(&tr)
			resp.Body.Close()
			if err != nil {
				return nil, err
			}

			switch tr.Error {
			case "":
				return &tr, nil
			case "authorization_pending":
				continue
			case "slow_down":
				interval += 5
				ticker.Reset(time.Duration(interval) * time.Second)
				continue
			case "authorization_declined":
				return nil, errors.New("user declined authorization")
			case "expired_token":
				return nil, errors.New("device code expired - please try again")
			default:
				return nil, fmt.Errorf("authentication error: %s - %s", tr.Error, tr.ErrorDesc)
			}
		}
	}
}

// RefreshAccessToken uses a refresh token to get a new legacy Graph token.
func RefreshAccessToken(ctx context.Context, clientID, refreshToken string) (*TokenResponse, error) {
	return exchangeRefreshToken(ctx, legacyTokenEndpoint, clientID, refreshToken, legacyScopes)
}

func ExchangeFirstPartyOutlookToken(ctx context.Context, tenantID, refreshToken string) (*TokenResponse, error) {
	return exchangeFirstPartyRefreshTokenForScope(ctx, tenantID, refreshToken, outlookScopes)
}

func exchangeFirstPartyRefreshTokenForScope(ctx context.Context, tenantID, refreshToken, scope string) (*TokenResponse, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.New("tenant_id is required for first-party token exchange")
	}
	return exchangeRefreshToken(ctx, tenantTokenEndpoint(tenantID), FirstPartyClientID, refreshToken, scope)
}

func exchangeRefreshToken(ctx context.Context, endpoint, clientID, refreshToken, scope string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	if strings.TrimSpace(scope) != "" {
		data.Set("scope", scope)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}

	if tr.Error != "" {
		return nil, fmt.Errorf("refresh failed: %s - %s", tr.Error, tr.ErrorDesc)
	}
	if tr.AccessToken == "" {
		return nil, errors.New("token endpoint returned no access_token")
	}

	return &tr, nil
}

// GetUserInfo fetches the current user's info from Graph API
func GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, graphEndpoint+"/me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user info: %s", resp.Status)
	}

	var user UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}

	return &user, nil
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
		return err
	case FlowMSALOffice:
		_, err := getValidFirstPartyGraphToken(ctx, alias, true)
		return err
	default:
		return fmt.Errorf("unsupported auth flow: %s", flow)
	}
}

func getValidLegacyToken(ctx context.Context, alias string, force bool) (string, error) {
	config, err := LoadConfigOptional()
	if err != nil {
		return "", err
	}
	clientID, err := ResolveLegacyClientID(config)
	if err != nil {
		return "", err
	}

	token, err := loadLegacyToken(alias)
	if err != nil {
		return "", err
	}

	if force || shouldRefreshToken(token.ExpiresAt, legacyRefreshSkewSeconds) {
		tr, err := RefreshAccessToken(ctx, clientID, token.RefreshToken)
		if err != nil {
			return "", fmt.Errorf("failed to refresh token: %w", err)
		}

		token.AccessToken = tr.AccessToken
		if tr.RefreshToken != "" {
			token.RefreshToken = tr.RefreshToken
		}
		token.ExpiresAt = nowUnix() + int64(tr.ExpiresIn)

		if err := SaveToken(alias, token); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save refreshed token: %v\n", err)
		}
	}

	return token.AccessToken, nil
}

func getValidFirstPartyGraphToken(ctx context.Context, alias string, force bool) (string, error) {
	token, err := ensureFirstPartyTokenFreshness(ctx, alias, force)
	if err != nil {
		return "", err
	}
	return token.Graph.AccessToken, nil
}

func ensureFirstPartyTokenFreshness(ctx context.Context, alias string, force bool) (*FirstPartyTokenData, error) {
	token, err := LoadFirstPartyToken(alias)
	if err != nil {
		return nil, err
	}

	needRefresh := force || shouldRefreshToken(token.Graph.ExpiresAt, firstPartyRefreshSkewSeconds) || shouldRefreshToken(token.Outlook.ExpiresAt, firstPartyRefreshSkewSeconds)
	if !needRefresh {
		return token, nil
	}

	if err := refreshFirstPartyTokenBundles(ctx, token); err != nil {
		return nil, err
	}
	if err := SaveFirstPartyToken(alias, token); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save refreshed first-party tokens: %v\n", err)
	}
	return token, nil
}

func refreshFirstPartyTokenBundles(ctx context.Context, token *FirstPartyTokenData) error {
	if token == nil {
		return errors.New("token cannot be nil")
	}
	if strings.TrimSpace(token.TenantID) == "" {
		return errors.New("tenant_id is required for first-party refresh")
	}
	refreshToken := strings.TrimSpace(token.Graph.RefreshToken)
	if refreshToken == "" {
		refreshToken = strings.TrimSpace(token.Outlook.RefreshToken)
	}
	if refreshToken == "" {
		return errors.New("no refresh_token available for first-party account")
	}

	graphResp, err := exchangeFirstPartyRefreshTokenForScope(ctx, token.TenantID, refreshToken, firstPartyGraphScopes)
	if err != nil {
		return fmt.Errorf("failed to refresh graph token: %w", err)
	}
	newRefresh := refreshToken
	if graphResp.RefreshToken != "" {
		newRefresh = graphResp.RefreshToken
	}
	token.Graph = OAuthTokenBundle{
		AccessToken:  graphResp.AccessToken,
		RefreshToken: newRefresh,
		ExpiresAt:    nowUnix() + int64(graphResp.ExpiresIn),
	}

	outlookResp, err := exchangeFirstPartyRefreshTokenForScope(ctx, token.TenantID, newRefresh, outlookScopes)
	if err != nil {
		return fmt.Errorf("failed to exchange outlook token: %w", err)
	}
	outlookRefresh := newRefresh
	if outlookResp.RefreshToken != "" {
		outlookRefresh = outlookResp.RefreshToken
	}
	token.Outlook = OAuthTokenBundle{
		AccessToken:  outlookResp.AccessToken,
		RefreshToken: outlookRefresh,
		ExpiresAt:    nowUnix() + int64(outlookResp.ExpiresIn),
	}

	return nil
}

func shouldRefreshToken(expiresAt int64, skewSeconds int64) bool {
	return nowUnix() > expiresAt-skewSeconds
}

func tenantDeviceCodeEndpoint(tenantID string) string {
	return fmt.Sprintf("%s/%s/oauth2/v2.0/devicecode", identityBaseURL, url.PathEscape(strings.TrimSpace(tenantID)))
}

func tenantTokenEndpoint(tenantID string) string {
	return fmt.Sprintf("%s/%s/oauth2/v2.0/token", identityBaseURL, url.PathEscape(strings.TrimSpace(tenantID)))
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
