package auth

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	acctPattern = regexp.MustCompile(`"acct"<blob>="([^"]+)"`)
	genaPattern = regexp.MustCompile(`"gena"<blob>=0x([0-9a-fA-F\s]+)`)
)

// OneAuthAccount is a discovered local OneAuth account from macOS keychain metadata.
type OneAuthAccount struct {
	Email    string `json:"email"`
	TenantID string `json:"tenant_id"`
}

func ParseOneAuthSecurityOutput(raw string) ([]OneAuthAccount, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("security output is empty")
	}

	records := strings.Split(raw, "keychain:")
	accounts := make([]OneAuthAccount, 0)
	seen := map[string]struct{}{}

	for _, record := range records {
		acctMatch := acctPattern.FindStringSubmatch(record)
		genaMatch := genaPattern.FindStringSubmatch(record)
		if len(acctMatch) < 2 || len(genaMatch) < 2 {
			continue
		}

		email := strings.TrimSpace(acctMatch[1])
		realm, err := extractRealmFromGenaHex(genaMatch[1])
		if err != nil {
			continue
		}
		if email == "" || realm == "" {
			continue
		}

		key := strings.ToLower(email) + "|" + realm
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		accounts = append(accounts, OneAuthAccount{Email: email, TenantID: realm})
	}

	if len(accounts) == 0 {
		return nil, errors.New("no OneAuth accounts found in macOS keychain output")
	}

	sort.Slice(accounts, func(i, j int) bool {
		return strings.ToLower(accounts[i].Email) < strings.ToLower(accounts[j].Email)
	})
	return accounts, nil
}

func extractRealmFromGenaHex(rawHex string) (string, error) {
	cleanedHex := strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9':
			return r
		case r >= 'a' && r <= 'f':
			return r
		case r >= 'A' && r <= 'F':
			return r
		default:
			return -1
		}
	}, rawHex)
	if cleanedHex == "" {
		return "", errors.New("gena value was not valid hex")
	}

	decoded, err := hex.DecodeString(cleanedHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode gena hex: %w", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return "", fmt.Errorf("failed to decode gena json: %w", err)
	}

	realm := findNestedString(payload, "realm")
	if realm == "" {
		realm = findNestedString(payload, "tenant_id")
	}
	if realm == "" {
		realm = findNestedString(payload, "tid")
	}
	if realm == "" {
		return "", errors.New("realm not found in OneAuth metadata")
	}
	return realm, nil
}

func findNestedString(v interface{}, key string) string {
	switch data := v.(type) {
	case map[string]interface{}:
		for k, value := range data {
			if strings.EqualFold(k, key) {
				if out, ok := value.(string); ok {
					return strings.TrimSpace(out)
				}
			}
			if nested := findNestedString(value, key); nested != "" {
				return nested
			}
		}
	case []interface{}:
		for _, item := range data {
			if nested := findNestedString(item, key); nested != "" {
				return nested
			}
		}
	}
	return ""
}
