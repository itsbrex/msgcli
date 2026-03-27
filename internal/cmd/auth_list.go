package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/spf13/cobra"
)

var authListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured accounts",
	RunE:  runAuthList,
}

func init() {
	authCmd.AddCommand(authListCmd)
}

func runAuthList(cmd *cobra.Command, args []string) error {
	accounts, err := auth.ListAccounts()
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}

	if len(accounts) == 0 {
		Infof("No accounts configured. Run 'msgcli auth add <alias>' to add one.")
		return nil
	}

	format := GetOutputFormat()
	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(accounts)
	}

	// Table format
	fmt.Printf("%-15s %-18s %s\n", "ALIAS", "FLOW", "EMAIL")
	fmt.Printf("%-15s %-18s %s\n", "-----", "----", "-----")
	for _, acc := range accounts {
		fmt.Printf("%-15s %-18s %s\n", acc.Alias, acc.Flow, acc.Email)
	}

	return nil
}
