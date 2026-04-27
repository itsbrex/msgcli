package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/spf13/cobra"
)

var (
	authAddFlow  string
	authAddEmail string
)

var authAddCmd = &cobra.Command{
	Use:   "add <alias>",
	Short: "Add a new account via legacy or msal-office authentication",
	Long: `Authenticate with a Microsoft account and store credentials for an alias.

The alias is a friendly name you choose (e.g., "personal", "work").
You can have multiple accounts and switch between them with --account.

Flow options:
- legacy: uses your configured Azure app client ID
- msal-office: uses Microsoft first-party client and macOS OneAuth realm metadata`,
	Example: `  msgcli auth add personal --flow legacy
  msgcli auth add work --flow msal-office --email you@company.com`,
	Args: cobra.ExactArgs(1),
	RunE: runAuthAdd,
}

func init() {
	authAddCmd.Flags().StringVar(&authAddFlow, "flow", "", "Auth flow to use: legacy or msal-office")
	authAddCmd.Flags().StringVar(&authAddEmail, "email", "", "Preferred account email for msal-office auto-selection")
	authCmd.AddCommand(authAddCmd)
}

func runAuthAdd(cmd *cobra.Command, args []string) error {
	alias := args[0]

	config, err := auth.LoadConfigOptional()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	flow, err := auth.ResolveAuthFlow(authAddFlow, config)
	if err != nil {
		return fmt.Errorf("resolve auth flow: %w", err)
	}

	// Check if account already exists
	existing, err := auth.LoadToken(alias)
	if err == nil && existing != nil {
		Infof("Account '%s' already exists (email: %s)", alias, existing.Email)
		Infof("Use 'msgcli auth remove %s' first to replace it", alias)
		return fmt.Errorf("account already exists")
	}

	switch flow {
	case auth.FlowLegacy:
		return runAuthAddLegacy(alias, config)
	case auth.FlowMSALOffice:
		return runAuthAddFirstParty(alias)
	default:
		return fmt.Errorf("unsupported auth flow: %s", flow)
	}
}

func runAuthAddLegacy(alias string, config *auth.Config) error {
	if IsNoInput() {
		return fmt.Errorf("legacy auth add requires interactive input - cannot use --no-input")
	}
	clientID, err := auth.ResolveLegacyClientID(config)
	if err != nil {
		return fmt.Errorf("resolve client id: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	Infof("Starting device code authentication (flow: legacy)...")

	// Start device code flow
	dcr, err := auth.StartDeviceCodeFlow(ctx, clientID)
	if err != nil {
		return fmt.Errorf("failed to start authentication: %w", err)
	}

	// Display instructions to user
	Infof("")
	Infof("To sign in, open a browser and go to:")
	Infof("  %s", dcr.VerificationURI)
	Infof("")
	Infof("Enter the code: %s", dcr.UserCode)
	Infof("")
	Infof("Waiting for authentication...")

	// Poll for token
	tr, err := auth.PollForToken(ctx, clientID, dcr.DeviceCode, dcr.Interval)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	Infof("Authentication successful!")

	// Get user info
	userInfo, err := auth.GetUserInfo(ctx, tr.AccessToken)
	if err != nil {
		Infof("Warning: couldn't fetch user info: %v", err)
		userInfo = &auth.UserInfo{UserPrincipalName: "unknown"}
	}

	// Determine email to store
	email := userInfo.Mail
	if email == "" {
		email = userInfo.UserPrincipalName
	}

	// Save token
	token := &auth.TokenData{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Unix() + int64(tr.ExpiresIn),
		Email:        email,
	}

	if err := auth.SaveToken(alias, token); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	Infof("Account '%s' added successfully", alias)
	Infof("Email: %s", email)
	return nil
}

func runAuthAddFirstParty(alias string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	accounts, discoverErr := auth.DiscoverOneAuthAccounts(ctx)

	var selectedAccount auth.OneAuthAccount
	if discoverErr != nil {
		if strings.TrimSpace(authAddEmail) == "" {
			return discoverErr
		}

		tenantID, tenantErr := auth.ResolveTenantIDFromEmail(ctx, authAddEmail)
		if tenantErr != nil {
			return fmt.Errorf("%v; and fallback tenant resolution from --email failed: %w", discoverErr, tenantErr)
		}
		selectedAccount = auth.OneAuthAccount{
			Email:    strings.TrimSpace(authAddEmail),
			TenantID: tenantID,
		}
		Infof("OneAuth metadata not found; resolved tenant from email domain for %s", selectedAccount.Email)
	} else {
		var selectErr error
		selectedAccount, selectErr = selectOneAuthAccount(accounts, authAddEmail)
		if selectErr != nil {
			return selectErr
		}
	}

	Infof("Using OneAuth account: %s (tenant: %s)", selectedAccount.Email, selectedAccount.TenantID)
	Infof("Starting device code authentication (flow: %s)...", auth.FlowMSALOffice)

	dcr, err := auth.StartFirstPartyDeviceCodeFlow(ctx, selectedAccount.TenantID)
	if err != nil {
		return fmt.Errorf("failed to start first-party authentication: %w", err)
	}

	if strings.TrimSpace(dcr.Message) != "" {
		Infof("")
		Infof(dcr.Message)
		Infof("")
	} else {
		Infof("")
		Infof("To sign in, open a browser and go to:")
		Infof("  %s", dcr.VerificationURI)
		Infof("")
		Infof("Enter the code: %s", dcr.UserCode)
		Infof("")
	}
	Infof("Waiting for authentication...")

	graphToken, err := auth.PollForFirstPartyGraphToken(ctx, selectedAccount.TenantID, dcr.DeviceCode, dcr.Interval)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	outlookToken, err := auth.ExchangeFirstPartyOutlookToken(ctx, selectedAccount.TenantID, graphToken.RefreshToken)
	if err != nil {
		return fmt.Errorf("failed to exchange outlook token: %w", err)
	}

	email := selectedAccount.Email
	if email == "" {
		userInfo, userErr := auth.GetUserInfo(ctx, graphToken.AccessToken)
		if userErr == nil {
			email = userInfo.Mail
			if email == "" {
				email = userInfo.UserPrincipalName
			}
		}
	}

	token := &auth.FirstPartyTokenData{
		Flow:     auth.FlowMSALOffice,
		TenantID: selectedAccount.TenantID,
		Email:    email,
		Graph: auth.OAuthTokenBundle{
			AccessToken:  graphToken.AccessToken,
			RefreshToken: fallbackRefreshToken(graphToken.RefreshToken, outlookToken.RefreshToken),
			ExpiresAt:    time.Now().Unix() + int64(graphToken.ExpiresIn),
		},
		Outlook: auth.OAuthTokenBundle{
			AccessToken:  outlookToken.AccessToken,
			RefreshToken: fallbackRefreshToken(outlookToken.RefreshToken, graphToken.RefreshToken),
			ExpiresAt:    time.Now().Unix() + int64(outlookToken.ExpiresIn),
		},
	}

	if err := auth.SaveFirstPartyToken(alias, token); err != nil {
		return fmt.Errorf("failed to save first-party credentials: %w", err)
	}

	Infof("Account '%s' added successfully", alias)
	Infof("Email: %s", email)
	Infof("Flow: %s", auth.FlowMSALOffice)
	return nil
}

func selectOneAuthAccount(accounts []auth.OneAuthAccount, email string) (auth.OneAuthAccount, error) {
	if len(accounts) == 0 {
		return auth.OneAuthAccount{}, fmt.Errorf("no OneAuth candidates found in macOS keychain")
	}

	if normalized := strings.ToLower(strings.TrimSpace(email)); normalized != "" {
		for _, candidate := range accounts {
			if strings.ToLower(candidate.Email) == normalized {
				return candidate, nil
			}
		}
		return auth.OneAuthAccount{}, fmt.Errorf("no OneAuth account matched --email %s", email)
	}

	if len(accounts) == 1 {
		return accounts[0], nil
	}

	if IsNoInput() {
		return auth.OneAuthAccount{}, fmt.Errorf("multiple OneAuth accounts found; rerun with --email user@domain or without --no-input")
	}

	Infof("Multiple OneAuth accounts found:")
	for idx, candidate := range accounts {
		Infof("  %d) %s (tenant: %s)", idx+1, candidate.Email, candidate.TenantID)
	}
	Infof("")
	fmt.Fprint(os.Stderr, "Select account number: ")
	reader := bufio.NewReader(os.Stdin)
	rawSelection, err := reader.ReadString('\n')
	if err != nil {
		return auth.OneAuthAccount{}, err
	}
	rawSelection = strings.TrimSpace(rawSelection)
	choice, err := strconv.Atoi(rawSelection)
	if err != nil || choice < 1 || choice > len(accounts) {
		return auth.OneAuthAccount{}, fmt.Errorf("invalid selection %q", rawSelection)
	}
	return accounts[choice-1], nil
}

func fallbackRefreshToken(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}
