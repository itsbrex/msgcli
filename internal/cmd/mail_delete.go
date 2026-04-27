package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/spf13/cobra"
)

var mailDeleteForce bool

var mailDeleteCmd = &cobra.Command{
	Use:   "delete <message-id> [<message-id>...]",
	Short: "Delete one or more email messages",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runMailDelete,
}

func init() {
	mailDeleteCmd.Flags().BoolVarP(&mailDeleteForce, "force", "f", false, "Skip confirmation prompt")
	mailCmd.AddCommand(mailDeleteCmd)
}

func runMailDelete(cmd *cobra.Command, args []string) error {
	account, err := auth.ResolveAccount(GetAccountFlag())
	if err != nil {
		return fmt.Errorf("resolve account: %w", err)
	}
	client := graph.NewClient(account)
	ctx := context.Background()

	if len(args) == 1 {
		return runMailDeleteSingle(ctx, client, args[0])
	}
	return runMailDeleteBulk(ctx, client, args)
}

func runMailDeleteSingle(ctx context.Context, client *graph.Client, messageID string) error {
	if !mailDeleteForce && !IsNoInput() {
		msg, err := client.GetMessage(ctx, messageID)
		if err != nil {
			return fmt.Errorf("failed to get message: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Delete message: %q? [y/N]: ", msg.Subject)
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			Infof("Cancelled")
			return nil
		}
	}
	if err := client.DeleteMessage(ctx, messageID); err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}
	Infof("Message deleted")
	return nil
}

func runMailDeleteBulk(ctx context.Context, client *graph.Client, ids []string) error {
	if !confirmBulk("Delete", len(ids), mailDeleteForce) {
		Infof("Cancelled")
		return nil
	}
	reqs := buildDeleteBatch(ids)
	responses, err := client.Batch(ctx, reqs)
	if err != nil {
		return fmt.Errorf("batch request: %w", err)
	}
	return reportBulkOutcome("Deleted", ids, responses)
}

func buildDeleteBatch(ids []string) []graph.BatchRequest {
	reqs := make([]graph.BatchRequest, len(ids))
	for i, id := range ids {
		reqs[i] = graph.BatchRequest{
			ID:     fmt.Sprintf("%d", i),
			Method: "DELETE",
			URL:    "/me/messages/" + id,
		}
	}
	return reqs
}
