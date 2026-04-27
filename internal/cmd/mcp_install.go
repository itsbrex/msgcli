package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var mcpInstallClient string

var mcpInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install msgcli into an MCP client's config",
	Long:  `Write an mcpServers.msgcli entry into a client's config file. Currently supports --client claude-code.`,
	RunE:  runMCPInstall,
}

func init() {
	mcpInstallCmd.Flags().StringVar(&mcpInstallClient, "client", "claude-code", "Target client: claude-code")
	mcpCmd.AddCommand(mcpInstallCmd)
}

func runMCPInstall(cmd *cobra.Command, args []string) error {
	if mcpInstallClient != "claude-code" {
		return fmt.Errorf("unsupported client %q (supported: claude-code)", mcpInstallClient)
	}
	bin, err := exec.LookPath("msgcli")
	if err != nil {
		// Fall back to absolute path of the running binary.
		if exe, e2 := os.Executable(); e2 == nil {
			bin = exe
		} else {
			return fmt.Errorf("cannot locate msgcli binary: %w", err)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	path := filepath.Join(home, ".claude", "settings.json")
	existing, _ := os.ReadFile(path) // treat missing as empty
	merged, err := mergeClaudeCodeMCPConfig(existing, bin)
	if err != nil {
		return fmt.Errorf("merge mcp config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, merged, 0o600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	Infof("Installed msgcli MCP server into %s", path)
	return nil
}

// mergeClaudeCodeMCPConfig adds/updates an mcpServers.msgcli entry without
// clobbering existing keys. Accepts empty input (returns a fresh config).
func mergeClaudeCodeMCPConfig(existing []byte, bin string) ([]byte, error) {
	root := map[string]interface{}{}
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &root); err != nil {
			return nil, fmt.Errorf("existing config is not valid JSON: %w", err)
		}
	}
	servers, _ := root["mcpServers"].(map[string]interface{})
	if servers == nil {
		servers = map[string]interface{}{}
	}
	servers["msgcli"] = map[string]interface{}{
		"command": bin,
		"args":    []string{"mcp", "serve"},
	}
	root["mcpServers"] = servers
	return json.MarshalIndent(root, "", "  ")
}
