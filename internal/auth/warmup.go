package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// WarmupResult is the per-account outcome of an auto-refresh attempt.
type WarmupResult struct {
	Alias string
	Flow  AuthFlow
	Email string
	Err   error
}

// WarmupAccounts ensures every configured account has a fresh access token.
// It is safe to call on every CLI startup: GetValidToken only hits the network
// when a token is within its refresh skew window, so warm tokens are a no-op.
// Errors are returned per-account; callers decide whether to log or fail.
func WarmupAccounts(ctx context.Context) []WarmupResult {
	accounts, err := ListAccounts()
	if err != nil || len(accounts) == 0 {
		return nil
	}

	results := make([]WarmupResult, len(accounts))
	var wg sync.WaitGroup
	for i, acc := range accounts {
		wg.Add(1)
		go func(i int, acc AccountInfo) {
			defer wg.Done()
			res := WarmupResult{Alias: acc.Alias, Flow: acc.Flow, Email: acc.Email}
			if _, err := GetValidToken(ctx, acc.Alias); err != nil {
				res.Err = err
			}
			results[i] = res
		}(i, acc)
	}
	wg.Wait()
	return results
}

// EnsureConfig creates ~/.msgcli/config.json with sensible defaults if it does
// not yet exist but accounts are already configured. The default flow is taken
// from the existing accounts (msal-office wins if any account uses it).
// No-op when config already exists or no accounts are configured.
func EnsureConfig() error {
	cfg, err := LoadConfigOptional()
	if err != nil {
		return err
	}
	if cfg != nil {
		return nil
	}

	accounts, err := ListAccounts()
	if err != nil || len(accounts) == 0 {
		return nil
	}

	defaultFlow := FlowLegacy
	for _, acc := range accounts {
		if acc.Flow == FlowMSALOffice {
			defaultFlow = FlowMSALOffice
			break
		}
	}

	newCfg := &Config{DefaultAuthFlow: defaultFlow}
	if err := SaveConfig(newCfg); err != nil {
		return fmt.Errorf("bootstrap config: %w", err)
	}
	return nil
}

// FormatWarmupErrors returns a single-line summary of any failed accounts,
// or empty string if all warmups succeeded. Useful for stderr logging.
func FormatWarmupErrors(results []WarmupResult) string {
	var failures []string
	for _, r := range results {
		if r.Err != nil && !errors.Is(r.Err, context.Canceled) {
			failures = append(failures, fmt.Sprintf("%s (%s): %v", r.Alias, r.Flow, r.Err))
		}
	}
	if len(failures) == 0 {
		return ""
	}
	return fmt.Sprintf("auto-refresh failed for %d account(s): %s", len(failures), failures)
}
