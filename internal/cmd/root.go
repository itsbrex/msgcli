package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/spf13/cobra"
)

var (
	accountFlag string
	outputFlag  string
	noInputFlag bool
)

// EnvDisableWarmup turns off the startup auto-refresh, e.g. for offline use.
const EnvDisableWarmup = "MSGCLI_DISABLE_WARMUP"

var rootCmd = &cobra.Command{
	Use:   "msgcli",
	Short: "Agent-first CLI for Microsoft Outlook Mail and Calendar",
	Long: `msgcli is a command-line interface for Microsoft Graph API,
focused on Outlook Mail and Calendar operations.

Designed for AI agents with JSON-first output, multi-account support,
and secure credential storage.`,
	PersistentPreRun: autoRefreshPreRun,
}

// commandsSkippingWarmup lists fully-qualified command paths that must not
// trigger a startup token refresh (auth management, install helpers, etc.).
var commandsSkippingWarmup = map[string]bool{
	"msgcli auth setup":   true,
	"msgcli auth add":     true,
	"msgcli auth remove":  true,
	"msgcli auth refresh": true,
	"msgcli mcp install":  true,
	"msgcli help":         true,
	"msgcli completion":   true,
}

func autoRefreshPreRun(cmd *cobra.Command, _ []string) {
	if os.Getenv(EnvDisableWarmup) != "" {
		return
	}
	path := cmd.CommandPath()
	for skip := range commandsSkippingWarmup {
		if path == skip || strings.HasPrefix(path, skip+" ") {
			return
		}
	}
	if err := auth.EnsureConfig(); err != nil {
		Infof("Warning: bootstrap config: %v", err)
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()
	results := auth.WarmupAccounts(ctx)
	if msg := auth.FormatWarmupErrors(results); msg != "" {
		Infof("Warning: %s", msg)
	}
}

func Execute() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	rootCmd.SetContext(ctx)
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&accountFlag, "account", "a", "", "Account alias to use (default: first configured)")
	rootCmd.PersistentFlags().StringVarP(&outputFlag, "output", "o", "", "Output format: json or table (default: auto)")
	rootCmd.PersistentFlags().BoolVar(&noInputFlag, "no-input", false, "Never prompt for input, fail if input needed")
}

// GetAccountFlag returns the account flag value
func GetAccountFlag() string {
	return accountFlag
}

// GetOutputFormat returns the output format, auto-detecting if not specified
func GetOutputFormat() string {
	if outputFlag != "" {
		return outputFlag
	}
	// Auto-detect: JSON for pipes, table for terminals
	fi, _ := os.Stdout.Stat()
	if (fi.Mode() & os.ModeCharDevice) == 0 {
		return "json"
	}
	return "table"
}

// IsNoInput returns whether no-input mode is enabled
func IsNoInput() bool {
	return noInputFlag
}

// Infof prints an info message to stderr (for progress, not data)
func Infof(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// newClientFromFlag resolves the account from the --account flag and returns
// a graph client and the command's context ready for use.
func newClientFromFlag(cmd *cobra.Command) (*graph.Client, context.Context, error) {
	account, err := auth.ResolveAccount(GetAccountFlag())
	if err != nil {
		return nil, nil, err
	}
	return graph.NewClient(account), cmd.Context(), nil
}

// writeJSON encodes v as indented JSON to stdout.
func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// readBodyFromStdin reads all lines from stdin and returns them joined by newlines,
// with the trailing newline stripped.
func readBodyFromStdin() (string, error) {
	var b strings.Builder
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		b.WriteString(sc.Text())
		b.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}
