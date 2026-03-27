package cmd

import (
	"testing"

	"github.com/skylarbpayne/msgcli/internal/auth"
)

func TestSelectOneAuthAccountByEmail(t *testing.T) {
	accounts := []auth.OneAuthAccount{
		{Email: "one@contoso.com", TenantID: "tenant-1"},
		{Email: "two@contoso.com", TenantID: "tenant-2"},
	}

	selected, err := selectOneAuthAccount(accounts, "two@contoso.com")
	if err != nil {
		t.Fatalf("selectOneAuthAccount returned error: %v", err)
	}
	if selected.Email != "two@contoso.com" {
		t.Fatalf("unexpected selected account: %+v", selected)
	}
}

func TestSelectOneAuthAccountNoInputMultipleFails(t *testing.T) {
	originalNoInput := noInputFlag
	noInputFlag = true
	t.Cleanup(func() { noInputFlag = originalNoInput })

	accounts := []auth.OneAuthAccount{
		{Email: "one@contoso.com", TenantID: "tenant-1"},
		{Email: "two@contoso.com", TenantID: "tenant-2"},
	}

	if _, err := selectOneAuthAccount(accounts, ""); err == nil {
		t.Fatalf("expected --no-input selection failure when multiple candidates exist")
	}
}
