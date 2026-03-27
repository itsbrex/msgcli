//go:build !darwin

package auth

import (
	"context"
	"fmt"
)

func DiscoverOneAuthAccounts(ctx context.Context) ([]OneAuthAccount, error) {
	_ = ctx
	return nil, fmt.Errorf("msal-office flow is only supported on macOS because it requires /usr/bin/security OneAuth metadata")
}
