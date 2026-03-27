package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/spf13/cobra"
)

var authRefreshFlow string

var authRefreshCmd = &cobra.Command{
	Use:   "refresh [alias]",
	Short: "Force refresh auth tokens for an account",
	Long: `Force-refresh stored tokens for an account alias.

Use --flow to target legacy or msal-office accounts explicitly.`,
	Example: `  msgcli auth refresh
  msgcli auth refresh work --flow msal-office
  msgcli auth refresh personal --flow legacy`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAuthRefresh,
}

func init() {
	authRefreshCmd.Flags().StringVar(&authRefreshFlow, "flow", "", "Auth flow to use: legacy or msal-office")
	authCmd.AddCommand(authRefreshCmd)
}

func runAuthRefresh(cmd *cobra.Command, args []string) error {
	accountInput := GetAccountFlag()
	if len(args) == 1 {
		accountInput = args[0]
	}

	alias, err := auth.ResolveAccount(accountInput)
	if err != nil {
		return err
	}

	config, err := auth.LoadConfigOptional()
	if err != nil {
		return err
	}

	flow, err := auth.ResolveAuthFlow(authRefreshFlow, config)
	if err != nil {
		return err
	}

	detectedFlow, detectErr := auth.DetectAccountFlow(alias)
	if detectErr == nil && detectedFlow != flow {
		return fmt.Errorf("account '%s' is configured for flow %s; rerun with --flow %s", alias, detectedFlow, detectedFlow)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := auth.RefreshAccountTokens(ctx, alias, flow); err != nil {
		return err
	}

	Infof("Account '%s' refreshed successfully (flow: %s)", alias, flow)
	return nil
}
