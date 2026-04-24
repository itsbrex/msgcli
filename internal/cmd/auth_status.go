package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/spf13/cobra"
)

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show authentication status",
	RunE:  runAuthStatus,
}

func init() {
	authCmd.AddCommand(authStatusCmd)
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	status, err := auth.Status(ctx, "")
	if err != nil {
		return err
	}

	format := GetOutputFormat()
	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	// Table format
	if !status.ConfigExists {
		fmt.Println("Configuration: NOT SET (default flow: legacy)")
	} else {
		if status.ClientID == "" {
			fmt.Printf("Configuration: OK (client_id: unset)\n")
		} else {
			fmt.Printf("Configuration: OK (client_id: %s)\n", status.ClientID)
		}
		fmt.Printf("Default flow: %s\n", status.DefaultFlow)
	}
	fmt.Println()

	if len(status.Accounts) == 0 {
		Infof("No accounts configured. Run 'msgcli auth add <alias>' to add one.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ALIAS\tFLOW\tEMAIL\tVALID\tEXPIRES")
	fmt.Fprintln(w, "-----\t----\t-----\t-----\t-------")
	for _, acc := range status.Accounts {
		validStr := "no"
		if acc.Valid {
			validStr = "yes"
		}
		expiry := acc.ExpiresAt
		if expiry == "" {
			expiry = "unknown"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", acc.Alias, acc.Flow, acc.Email, validStr, expiry)
		if acc.Error != "" {
			fmt.Fprintf(w, "\t\t\t\tError: %s\n", acc.Error)
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}

	return nil
}
