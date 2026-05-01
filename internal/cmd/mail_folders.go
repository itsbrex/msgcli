package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var mailFoldersCmd = &cobra.Command{
	Use:   "folders",
	Short: "List mail folders",
	RunE:  runMailFolders,
}

func init() {
	mailCmd.AddCommand(mailFoldersCmd)
}

func runMailFolders(cmd *cobra.Command, args []string) error {
	client, ctx, err := newClientFromFlag(cmd)
	if err != nil {
		return fmt.Errorf("resolve account: %w", err)
	}

	result, err := client.ListMailFolders(ctx)
	if err != nil {
		return fmt.Errorf("failed to list folders: %w", err)
	}

	format := GetOutputFormat()
	if format == "json" {
		return writeJSON(result.Value)
	}

	// Table format
	fmt.Printf("%-40s %-20s %8s %8s\n", "ID", "NAME", "UNREAD", "TOTAL")
	fmt.Printf("%-40s %-20s %8s %8s\n", "----", "----", "------", "-----")
	for _, folder := range result.Value {
		id := folder.ID
		if len(id) > 40 {
			id = id[:37] + "..."
		}
		fmt.Printf("%-40s %-20s %8d %8d\n", id, folder.DisplayName, folder.UnreadItemCount, folder.TotalItemCount)
	}

	return nil
}
