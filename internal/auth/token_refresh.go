package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ExchangeFirstPartyOutlookToken exchanges a first-party refresh token for an Outlook-scoped token.
func ExchangeFirstPartyOutlookToken(ctx context.Context, tenantID, refreshToken string) (*TokenResponse, error) {
	return exchangeFirstPartyRefreshTokenForScope(ctx, tenantID, refreshToken, outlookScopes)
}

func exchangeFirstPartyRefreshTokenForScope(ctx context.Context, tenantID, refreshToken, scope string) (*TokenResponse, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.New("tenant_id is required for first-party token exchange")
	}
	return exchangeRefreshToken(ctx, tenantTokenEndpoint(tenantID), FirstPartyClientID, refreshToken, scope)
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
