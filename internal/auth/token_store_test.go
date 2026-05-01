package auth

import (
	"io/fs"
	"os"
	"testing"
)

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

func TestSaveFirstPartyTokenEnforcesPermissions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	alias := "work"
	input := &FirstPartyTokenData{
		Flow:     FlowMSALOffice,
		TenantID: "tenant-123",
		Email:    "person@contoso.com",
		Graph: OAuthTokenBundle{
			AccessToken:  "graph-access",
			RefreshToken: "graph-refresh",
			ExpiresAt:    2000,
		},
		Outlook: OAuthTokenBundle{
			AccessToken:  "outlook-access",
			RefreshToken: "outlook-refresh",
			ExpiresAt:    2000,
		},
	}

	if err := SaveFirstPartyToken(alias, input); err != nil {
		t.Fatalf("SaveFirstPartyToken failed: %v", err)
	}

	dirPerm, filePerm, err := tokenStorePermissions(alias)
	if err != nil {
		t.Fatalf("tokenStorePermissions failed: %v", err)
	}
	if dirPerm != 0700 {
		t.Fatalf("expected token dir mode 0700, got %#o", dirPerm)
	}
	if filePerm != 0600 {
		t.Fatalf("expected token file mode 0600, got %#o", filePerm)
	}

	loaded, err := LoadFirstPartyToken(alias)
	if err != nil {
		t.Fatalf("LoadFirstPartyToken failed: %v", err)
	}
	if loaded.TenantID != input.TenantID || loaded.Email != input.Email {
		t.Fatalf("loaded token mismatch: %+v", loaded)
	}
}
