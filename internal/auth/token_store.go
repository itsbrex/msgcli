package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	tokenDirName = "tokens"
)

// OAuthTokenBundle stores access + refresh token material and expiry.
type OAuthTokenBundle struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

// FirstPartyTokenData stores dual Graph/Outlook tokens for msal-office flow.
type FirstPartyTokenData struct {
	Flow     AuthFlow         `json:"flow"`
	TenantID string           `json:"tenant_id"`
	Email    string           `json:"email"`
	Graph    OAuthTokenBundle `json:"graph"`
	Outlook  OAuthTokenBundle `json:"outlook"`
}

func tokenStoreDir() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, tokenDirName), nil
}

func validateAlias(alias string) error {
	if strings.TrimSpace(alias) == "" {
		return errors.New("alias cannot be empty")
	}
	if strings.Contains(alias, "/") || strings.Contains(alias, "\\") {
		return errors.New("alias cannot contain path separators")
	}
	return nil
}

func firstPartyTokenPath(alias string) (string, error) {
	if err := validateAlias(alias); err != nil {
		return "", err
	}
	dir, err := tokenStoreDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, alias+".json"), nil
}

func ensureTokenStoreDir() (string, error) {
	dir, err := tokenStoreDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

func SaveFirstPartyToken(alias string, token *FirstPartyTokenData) error {
	if token == nil {
		return errors.New("token cannot be nil")
	}
	if err := validateAlias(alias); err != nil {
		return err
	}
	if token.Flow == "" {
		token.Flow = FlowMSALOffice
	}
	if token.Flow != FlowMSALOffice {
		return fmt.Errorf("invalid flow for first-party token: %s", token.Flow)
	}

	dir, err := ensureTokenStoreDir()
	if err != nil {
		return fmt.Errorf("ensure token store dir: %w", err)
	}

	payload, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}

	target := filepath.Join(dir, alias+".json")
	temp := target + ".tmp"
	if err := os.WriteFile(temp, payload, 0600); err != nil {
		return fmt.Errorf("write token file %s: %w", temp, err)
	}
	if err := os.Chmod(temp, 0600); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("chmod token file %s: %w", temp, err)
	}
	if err := os.Rename(temp, target); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("rename token file to %s: %w", target, err)
	}
	return os.Chmod(target, 0600)
}

func LoadFirstPartyToken(alias string) (*FirstPartyTokenData, error) {
	path, err := firstPartyTokenPath(alias)
	if err != nil {
		return nil, err
	}

	payload, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: run 'msgcli auth add %s --flow msal-office' first", ErrAccountNotFound, alias)
		}
		return nil, err
	}

	var token FirstPartyTokenData
	if err := json.Unmarshal(payload, &token); err != nil {
		return nil, err
	}
	if token.Flow == "" {
		token.Flow = FlowMSALOffice
	} else {
		normalizedFlow, err := ParseAuthFlow(token.Flow.String())
		if err != nil {
			return nil, fmt.Errorf("unexpected flow in token file: %s", token.Flow)
		}
		token.Flow = normalizedFlow
	}
	if strings.TrimSpace(token.TenantID) == "" {
		return nil, errors.New("tenant_id missing from token file")
	}
	return &token, nil
}

func DeleteFirstPartyToken(alias string) error {
	path, err := firstPartyTokenPath(alias)
	if err != nil {
		return fmt.Errorf("resolve token path: %w", err)
	}
	err = os.Remove(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: account '%s' not found", ErrAccountNotFound, alias)
		}
		return fmt.Errorf("delete token file %s: %w", path, err)
	}
	return nil
}

func FirstPartyTokenExists(alias string) bool {
	path, err := firstPartyTokenPath(alias)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func ListFirstPartyAccounts() ([]AccountInfo, error) {
	dir, err := tokenStoreDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []AccountInfo{}, nil
		}
		return nil, err
	}

	accounts := make([]AccountInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		alias := strings.TrimSuffix(entry.Name(), ".json")
		token, err := LoadFirstPartyToken(alias)
		if err != nil {
			continue
		}
		accounts = append(accounts, AccountInfo{
			Alias:    alias,
			Email:    token.Email,
			Flow:     FlowMSALOffice,
			TenantID: token.TenantID,
		})
	}

	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].Alias < accounts[j].Alias
	})
	return accounts, nil
}

func tokenStorePermissions(alias string) (fs.FileMode, fs.FileMode, error) {
	dir, err := tokenStoreDir()
	if err != nil {
		return 0, 0, err
	}
	filePath, err := firstPartyTokenPath(alias)
	if err != nil {
		return 0, 0, err
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		return 0, 0, err
	}
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return 0, 0, err
	}
	return dirInfo.Mode().Perm(), fileInfo.Mode().Perm(), nil
}
