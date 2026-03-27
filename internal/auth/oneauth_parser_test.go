package auth

import (
	"encoding/hex"
	"fmt"
	"testing"
)

func TestParseOneAuthSecurityOutput(t *testing.T) {
	realmJSON := []byte(`{"realm":"11111111-2222-3333-4444-555555555555"}`)
	genaHex := hex.EncodeToString(realmJSON)

	output := fmt.Sprintf(`keychain: "/Users/test/Library/Keychains/login.keychain-db"
class: "genp"
attributes:
    "acct"<blob>="person@contoso.com"
    "gena"<blob>=0x%s
`, genaHex)

	accounts, err := ParseOneAuthSecurityOutput(output)
	if err != nil {
		t.Fatalf("ParseOneAuthSecurityOutput returned error: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	if accounts[0].Email != "person@contoso.com" {
		t.Fatalf("unexpected email: %s", accounts[0].Email)
	}
	if accounts[0].TenantID != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("unexpected tenant: %s", accounts[0].TenantID)
	}
}

func TestExtractRealmFromGenaHexErrors(t *testing.T) {
	if _, err := extractRealmFromGenaHex("not-hex"); err == nil {
		t.Fatalf("expected invalid hex error")
	}

	nonRealmJSON := []byte(`{"foo":"bar"}`)
	if _, err := extractRealmFromGenaHex(hex.EncodeToString(nonRealmJSON)); err == nil {
		t.Fatalf("expected missing realm error")
	}
}
