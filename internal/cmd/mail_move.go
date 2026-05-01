package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/spf13/cobra"
)

var mailMoveFolder string

var mailMoveCmd = &cobra.Command{
	Use:   "move <message-id> [<message-id>...]",
	Short: "Move one or more email messages to a different folder",
	Long: `Move email messages to a different folder.

Use folder ID or well-known name (inbox, drafts, sentitems, deleteditems, archive, junkemail).`,
	Args: cobra.MinimumNArgs(1),
	RunE: runMailMove,
}

func init() {
	mailMoveCmd.Flags().StringVar(&mailMoveFolder, "folder", "", "Destination folder ID or name (required)")
	_ = mailMoveCmd.MarkFlagRequired("folder")
	mailCmd.AddCommand(mailMoveCmd)
}

func runMailMove(cmd *cobra.Command, args []string) error {
	client, ctx, err := newClientFromFlag(cmd)
	if err != nil {
		return fmt.Errorf("resolve account: %w", err)
	}

	if len(args) == 1 {
		return runMailMoveSingle(ctx, client, args[0])
	}
	return runMailMoveBulk(ctx, client, args)
}

func runMailMoveSingle(ctx context.Context, client *graph.Client, messageID string) error {
	result, err := client.MoveMessage(ctx, messageID, mailMoveFolder)
	if err != nil {
		return fmt.Errorf("failed to move message: %w", err)
	}
	if GetOutputFormat() == "json" {
		return writeJSON(result)
	}
	Infof("Message moved to folder: %s", result.ParentFolderID)
	return nil
}

func runMailMoveBulk(ctx context.Context, client *graph.Client, ids []string) error {
	reqs := buildMoveBatch(ids, mailMoveFolder)
	responses, err := client.Batch(ctx, reqs)
	if err != nil {
		return fmt.Errorf("batch request: %w", err)
	}

	if GetOutputFormat() == "json" {
		type moveOutcome struct {
			ID     string         `json:"id"`
			Status int            `json:"status"`
			Moved  *graph.Message `json:"moved,omitempty"`
			Error  string         `json:"error,omitempty"`
		}
		outcomes := make([]moveOutcome, len(responses))
		for i, r := range responses {
			outcomes[i].ID = ids[i]
			outcomes[i].Status = r.Status
			if r.OK() {
				var m graph.Message
				_ = json.Unmarshal(r.Body, &m)
				outcomes[i].Moved = &m
			} else {
				outcomes[i].Error = string(r.Body)
			}
		}
		return writeJSON(outcomes)
	}

	return reportBulkOutcome("Moved", ids, responses)
}

func buildMoveBatch(ids []string, destFolder string) []graph.BatchRequest {
	reqs := make([]graph.BatchRequest, len(ids))
	for i, id := range ids {
		reqs[i] = graph.BatchRequest{
			ID:      fmt.Sprintf("%d", i),
			Method:  "POST",
			URL:     "/me/messages/" + id + "/move",
			Body:    map[string]string{"destinationId": destFolder},
			Headers: map[string]string{"Content-Type": "application/json"},
		}
	}
	return reqs
}
