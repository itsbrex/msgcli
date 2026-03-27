package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/99designs/keyring"
)

const (
	serviceName = "msgcli"
)

var (
	// ErrAccountNotFound indicates the requested alias does not exist in any token store.
	ErrAccountNotFound = errors.New("account not found")
	keyringOpen        = openKeyring
)

// TokenData holds OAuth tokens for an account
type TokenData struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"` // Unix timestamp
	Email        string `json:"email"`
}

// AccountInfo holds metadata about a configured account
type AccountInfo struct {
	Alias    string   `json:"alias"`
	Email    string   `json:"email"`
	Flow     AuthFlow `json:"flow"`
	TenantID string   `json:"tenant_id,omitempty"`
}

func openKeyring() (keyring.Keyring, error) {
	return keyring.Open(keyring.Config{
		ServiceName: serviceName,
		// Use default backends for each platform:
		// macOS: Keychain
		// Linux: Secret Service or file-based fallback
		// Windows: Credential Manager
		AllowedBackends: []keyring.BackendType{
			keyring.KeychainBackend,
			keyring.SecretServiceBackend,
			keyring.WinCredBackend,
			keyring.FileBackend,
		},
		FileDir:                        "~/.msgcli/keyring",
		FilePasswordFunc:               fileKeyringPassword,
		KeychainTrustApplication:       true,
		KeychainAccessibleWhenUnlocked: true,
	})
}

func fileKeyringPassword(prompt string) (string, error) {
	// Check for environment variable first (for CI/agent use)
	if pw := getEnvPassword(); pw != "" {
		return pw, nil
	}
	return "", errors.New("file keyring requires MSGCLI_KEYRING_PASSWORD environment variable")
}

func getEnvPassword() string {
	return getEnv("MSGCLI_KEYRING_PASSWORD")
}

func getEnv(key string) string {
	// Using os.Getenv but wrapped for testability
	return envGetter(key)
}

var envGetter = os.Getenv

// SaveToken stores tokens for an account in the keyring
func SaveToken(alias string, token *TokenData) error {
	kr, err := keyringOpen()
	if err != nil {
		return err
	}

	data, err := json.Marshal(token)
	if err != nil {
		return err
	}

	return kr.Set(keyring.Item{
		Key:  "token:" + alias,
		Data: data,
	})
}

// LoadToken retrieves tokens for an account from the keyring
func LoadToken(alias string) (*TokenData, error) {
	legacyToken, err := loadLegacyToken(alias)
	if err == nil {
		return legacyToken, nil
	}
	if !errors.Is(err, ErrAccountNotFound) {
		return nil, err
	}

	firstPartyToken, fpErr := LoadFirstPartyToken(alias)
	if fpErr != nil {
		return nil, fpErr
	}
	return &TokenData{
		AccessToken:  firstPartyToken.Graph.AccessToken,
		RefreshToken: firstPartyToken.Graph.RefreshToken,
		ExpiresAt:    firstPartyToken.Graph.ExpiresAt,
		Email:        firstPartyToken.Email,
	}, nil
}

func loadLegacyToken(alias string) (*TokenData, error) {
	kr, err := keyringOpen()
	if err != nil {
		return nil, err
	}

	item, err := kr.Get("token:" + alias)
	if err != nil {
		if err == keyring.ErrKeyNotFound {
			return nil, fmt.Errorf("%w: run 'msgcli auth add %s --flow legacy' first", ErrAccountNotFound, alias)
		}
		return nil, err
	}

	var token TokenData
	if err := json.Unmarshal(item.Data, &token); err != nil {
		return nil, err
	}

	return &token, nil
}

// DeleteToken removes tokens for an account from the keyring
func DeleteToken(alias string) error {
	var deleted bool
	var firstErr error

	if err := deleteLegacyToken(alias); err == nil {
		deleted = true
	} else if !errors.Is(err, ErrAccountNotFound) {
		firstErr = err
	}

	if err := DeleteFirstPartyToken(alias); err == nil {
		deleted = true
	} else if !errors.Is(err, ErrAccountNotFound) && firstErr == nil {
		firstErr = err
	}

	if deleted {
		return nil
	}
	if firstErr != nil {
		return firstErr
	}
	return fmt.Errorf("%w: account '%s' not found", ErrAccountNotFound, alias)
}

func deleteLegacyToken(alias string) error {
	kr, err := keyringOpen()
	if err != nil {
		return err
	}

	if err := kr.Remove("token:" + alias); err != nil {
		if err == keyring.ErrKeyNotFound {
			return fmt.Errorf("%w: account '%s' not found", ErrAccountNotFound, alias)
		}
		return err
	}
	return nil
}

// ListAccounts returns all configured account aliases
func ListAccounts() ([]AccountInfo, error) {
	legacyAccounts, legacyErr := listLegacyAccounts()
	firstPartyAccounts, firstPartyErr := ListFirstPartyAccounts()

	if legacyErr != nil && firstPartyErr != nil {
		return nil, fmt.Errorf("failed to list accounts: legacy=%v firstparty=%v", legacyErr, firstPartyErr)
	}

	accountsByAlias := map[string]AccountInfo{}
	for _, acc := range legacyAccounts {
		accountsByAlias[acc.Alias] = acc
	}
	for _, acc := range firstPartyAccounts {
		accountsByAlias[acc.Alias] = acc
	}

	accounts := make([]AccountInfo, 0, len(accountsByAlias))
	for _, acc := range accountsByAlias {
		accounts = append(accounts, acc)
	}
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].Alias < accounts[j].Alias
	})
	return accounts, nil
}

// GetDefaultAccount returns the first configured account alias
func GetDefaultAccount() (string, error) {
	accounts, err := ListAccounts()
	if err != nil {
		return "", err
	}
	if len(accounts) == 0 {
		return "", errors.New("no accounts configured - run 'msgcli auth add <alias>' first")
	}
	return accounts[0].Alias, nil
}

func listLegacyAccounts() ([]AccountInfo, error) {
	kr, err := keyringOpen()
	if err != nil {
		return nil, err
	}

	keys, err := kr.Keys()
	if err != nil {
		return nil, err
	}

	accounts := make([]AccountInfo, 0, len(keys))
	for _, key := range keys {
		if len(key) <= 6 || key[:6] != "token:" {
			continue
		}
		alias := key[6:]
		item, err := kr.Get(key)
		if err != nil {
			continue
		}
		var token TokenData
		if err := json.Unmarshal(item.Data, &token); err != nil {
			continue
		}
		accounts = append(accounts, AccountInfo{
			Alias: alias,
			Email: token.Email,
			Flow:  FlowLegacy,
		})
	}
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].Alias < accounts[j].Alias
	})
	return accounts, nil
}
