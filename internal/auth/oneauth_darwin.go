//go:build darwin

package auth

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

const securityBinaryPath = "/usr/bin/security"

var oneAuthSecurityRunner = runSecurityCommand

func DiscoverOneAuthAccounts(ctx context.Context) ([]OneAuthAccount, error) {
	commands := [][]string{
		{"find-generic-password", "-s", "com.microsoft.OneAuth", "-g"},
		{"find-generic-password", "-s", "OneAuth", "-g"},
		{"find-generic-password", "-s", "com.microsoft.adalcache", "-g"},
		{"find-generic-password", "-s", "Microsoft Office Identities Cache 2", "-g"},
	}

	for _, args := range commands {
		cmdCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		output, err := oneAuthSecurityRunner(cmdCtx, args...)
		cancel()
		if err != nil {
			continue
		}
		accounts, parseErr := ParseOneAuthSecurityOutput(output)
		if parseErr == nil {
			return accounts, nil
		}
	}

	return nil, fmt.Errorf("failed to discover OneAuth realm metadata from targeted keychain entries; open Outlook/Office first and retry with --email")
}

func runSecurityCommand(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, securityBinaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("security command failed: %w", err)
	}
	return string(output), nil
}
