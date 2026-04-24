package cmd

import (
	"context"
	"os"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/skylarbpayne/msgcli/internal/mcp"
	"github.com/spf13/cobra"
)

// version is injected via ldflags in release builds; "dev" otherwise.
var version = "dev"

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Model Context Protocol integration",
}

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run msgcli as an MCP server over stdio",
	Long: `Start a Model Context Protocol server on stdin/stdout.

Intended to be launched by an MCP client (Claude Code, Claude Desktop, etc.);
see 'msgcli mcp install' to configure a client automatically.`,
	RunE: runMCPServe,
}

func init() {
	mcpCmd.AddCommand(mcpServeCmd)
	rootCmd.AddCommand(mcpCmd)
}

func runMCPServe(cmd *cobra.Command, args []string) error {
	cf := func(account string) (*graph.Client, error) {
		resolved, err := auth.ResolveAccount(account)
		if err != nil {
			return nil, err
		}
		return graph.NewClient(resolved), nil
	}

	reg := mcp.NewRegistry()
	mcp.RegisterMailTools(reg, cf)
	mcp.RegisterCalendarTools(reg, cf)
	mcp.RegisterAuthTools(reg)

	server := mcp.NewServer(reg, os.Stdin, os.Stdout, mcp.ServerInfo{Name: "msgcli", Version: version})
	return server.Run(context.Background())
}
