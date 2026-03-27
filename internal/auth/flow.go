package auth

import (
	"errors"
	"fmt"
	"strings"
)

const (
	EnvAuthFlow       = "MSGCLI_AUTH_FLOW"
	EnvClientID       = "MSGCLI_CLIENT_ID"
	FlowLegacyRaw     = "legacy"
	FlowMSALOfficeRaw = "msal-office"
	FlowMSALLegacyRaw = "msal-firstparty"

	// FirstPartyClientID is Microsoft's first-party Office client ID.
	FirstPartyClientID = "d3590ed6-52b3-4102-aeff-aad2292ab01c"
)

// AuthFlow defines the authentication flow type.
type AuthFlow string

const (
	FlowLegacy     AuthFlow = FlowLegacyRaw
	FlowMSALOffice AuthFlow = FlowMSALOfficeRaw
)

func (f AuthFlow) String() string {
	return string(f)
}

func (f AuthFlow) Validate() error {
	switch f {
	case FlowLegacy, FlowMSALOffice:
		return nil
	default:
		return fmt.Errorf("invalid auth flow %q (expected %q or %q)", f, FlowLegacy, FlowMSALOffice)
	}
}

func ParseAuthFlow(raw string) (AuthFlow, error) {
	normalized := strings.TrimSpace(strings.ToLower(raw))
	if normalized == FlowMSALLegacyRaw {
		normalized = FlowMSALOfficeRaw
	}
	flow := AuthFlow(normalized)
	if flow == "" {
		return "", errors.New("auth flow cannot be empty")
	}
	if err := flow.Validate(); err != nil {
		return "", err
	}
	return flow, nil
}

// ResolveAuthFlow resolves the selected flow in this order:
// flag > MSGCLI_AUTH_FLOW > config.default_auth_flow > legacy.
func ResolveAuthFlow(flagFlow string, cfg *Config) (AuthFlow, error) {
	if strings.TrimSpace(flagFlow) != "" {
		return ParseAuthFlow(flagFlow)
	}

	if raw := strings.TrimSpace(getEnv(EnvAuthFlow)); raw != "" {
		return ParseAuthFlow(raw)
	}

	if cfg != nil && strings.TrimSpace(cfg.DefaultAuthFlow.String()) != "" {
		return ParseAuthFlow(cfg.DefaultAuthFlow.String())
	}

	return FlowLegacy, nil
}

func ResolveLegacyClientID(cfg *Config) (string, error) {
	if envClientID := strings.TrimSpace(getEnv(EnvClientID)); envClientID != "" {
		return envClientID, nil
	}
	if cfg != nil {
		if configClientID := strings.TrimSpace(cfg.ClientID); configClientID != "" {
			return configClientID, nil
		}
	}
	return "", errors.New("client_id not configured - run 'msgcli auth setup' first or set MSGCLI_CLIENT_ID")
}
