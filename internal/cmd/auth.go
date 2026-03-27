package cmd

import (
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication and accounts",
	Long: `Commands to configure and manage authentication.

Supported auth flows:
- legacy: custom Azure app registration + keyring-backed tokens
- msal-office: Microsoft first-party client + macOS OneAuth tenant discovery`,
}

func init() {
	rootCmd.AddCommand(authCmd)
}
