package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

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

func tenantDeviceCodeEndpoint(tenantID string) string {
	return fmt.Sprintf("%s/%s/oauth2/v2.0/devicecode", identityBaseURL, url.PathEscape(strings.TrimSpace(tenantID)))
}

func tenantTokenEndpoint(tenantID string) string {
	return fmt.Sprintf("%s/%s/oauth2/v2.0/token", identityBaseURL, url.PathEscape(strings.TrimSpace(tenantID)))
}
