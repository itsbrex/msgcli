package auth

import (
	"context"
	"time"
)

// StatusReport is the structured auth-status snapshot returned by Status.
type StatusReport struct {
	ConfigExists bool            `json:"config_exists"`
	ClientID     string          `json:"client_id,omitempty"`
	DefaultFlow  string          `json:"default_auth_flow,omitempty"`
	Accounts     []AccountStatus `json:"accounts"`
}

// AccountStatus is per-account auth status.
type AccountStatus struct {
	Alias     string `json:"alias"`
	Flow      string `json:"flow"`
	Email     string `json:"email"`
	ExpiresAt string `json:"expires_at"`
	Valid     bool   `json:"valid"`
	Error     string `json:"error,omitempty"`
}

// Status returns the current auth configuration and per-account validity.
// If alias is non-empty, only that account is reported; empty alias reports all.
func Status(ctx context.Context, alias string) (*StatusReport, error) {
	report := &StatusReport{}

	config, err := LoadConfigOptional()
	if err != nil {
		report.ConfigExists = false
	} else if config != nil {
		report.ConfigExists = true
		report.DefaultFlow = config.DefaultAuthFlow.String()
		if config.ClientID != "" {
			if len(config.ClientID) > 8 {
				report.ClientID = config.ClientID[:8] + "..."
			} else {
				report.ClientID = config.ClientID
			}
		}
	}

	accounts, err := ListAccounts()
	if err != nil {
		accounts = nil
	}

	for _, acc := range accounts {
		if alias != "" && acc.Alias != alias {
			continue
		}
		as := AccountStatus{Alias: acc.Alias, Flow: acc.Flow.String(), Email: acc.Email}
		token, err := LoadToken(acc.Alias)
		if err != nil {
			as.Error = err.Error()
		} else {
			as.ExpiresAt = time.Unix(token.ExpiresAt, 0).Local().Format("Jan 02 15:04 MST")
			if _, err := GetValidToken(ctx, acc.Alias); err != nil {
				as.Valid = false
				as.Error = err.Error()
			} else {
				as.Valid = true
			}
		}
		report.Accounts = append(report.Accounts, as)
	}

	return report, nil
}
