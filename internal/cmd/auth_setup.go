package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/spf13/cobra"
)

var authSetupDefaultFlow string

var authSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure auth defaults (legacy client ID and/or default flow)",
	Long: `Configure auth defaults for msgcli.

Legacy flow setup requires an Azure app registration client ID:

You need to create an Azure app registration first:
1. Go to https://portal.azure.com → App registrations → New registration
2. Name: "msgcli" (or anything)
3. Supported account types: "Accounts in any organizational directory and personal Microsoft accounts"
4. Click Register
5. Copy the "Application (client) ID"
6. Go to Authentication → Enable "Allow public client flows" → Save
7. Run this command and paste the client ID

If you prefer msal-office as the default flow, set --default-flow msal-office.`,
	Example: `  msgcli auth setup --default-flow legacy
  MSGCLI_CLIENT_ID=<your-client-id> msgcli auth setup --no-input --default-flow legacy
  msgcli auth setup --default-flow msal-office`,
	RunE: runAuthSetup,
}

func init() {
	authSetupCmd.Flags().StringVar(&authSetupDefaultFlow, "default-flow", "", "Default auth flow: legacy or msal-office")
	authCmd.AddCommand(authSetupCmd)
}

func runAuthSetup(cmd *cobra.Command, args []string) error {
	var existing *auth.Config
	var err error
	// Check if already configured
	existing, err = auth.LoadConfigOptional()
	if err != nil {
		return err
	}
	if existing != nil && existing.ClientID != "" {
		Infof("Current client ID: %s", existing.ClientID)
		if IsNoInput() {
			// no prompt path
		} else {
			fmt.Fprint(os.Stderr, "Replace existing configuration? [y/N]: ")
			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))
			if response != "y" && response != "yes" {
				Infof("Keeping existing configuration")
				return nil
			}
		}
	}

	// Check for environment variable first
	clientID := strings.TrimSpace(os.Getenv(auth.EnvClientID))

	if IsNoInput() {
		if clientID == "" && existing != nil {
			clientID = strings.TrimSpace(existing.ClientID)
		}
	} else if clientID == "" {
		fmt.Fprint(os.Stderr, "Enter your Azure app client ID: ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		clientID = strings.TrimSpace(input)
	}

	defaultFlow := auth.FlowLegacy
	if existing != nil && existing.DefaultAuthFlow != "" {
		defaultFlow = existing.DefaultAuthFlow
	}
	if strings.TrimSpace(authSetupDefaultFlow) != "" {
		parsedFlow, err := auth.ParseAuthFlow(authSetupDefaultFlow)
		if err != nil {
			return err
		}
		defaultFlow = parsedFlow
	}

	if clientID == "" && defaultFlow == auth.FlowLegacy {
		if IsNoInput() {
			return fmt.Errorf("--no-input requires MSGCLI_CLIENT_ID when default flow is legacy")
		}
		return fmt.Errorf("client ID cannot be empty")
	}

	if clientID != "" {
		// Basic validation (GUID format)
		if len(clientID) != 36 || strings.Count(clientID, "-") != 4 {
			Infof("Warning: client ID doesn't look like a valid GUID")
		}
	}

	config := &auth.Config{
		ClientID:        clientID,
		DefaultAuthFlow: defaultFlow,
	}

	if err := auth.SaveConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	Infof("Configuration saved successfully")
	Infof("Default auth flow: %s", defaultFlow)
	if clientID == "" {
		Infof("Legacy flow still requires MSGCLI_CLIENT_ID or a configured client_id.")
	}
	Infof("Next: run 'msgcli auth add <alias>' to add an account")
	return nil
}
