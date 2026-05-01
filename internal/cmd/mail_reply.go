package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	mailReplyBody  string
	mailReplyAll   bool
	mailReplyStdin bool
)

var mailReplyCmd = &cobra.Command{
	Use:   "reply <message-id>",
	Short: "Reply to an email message",
	Args:  cobra.ExactArgs(1),
	RunE:  runMailReply,
}

func init() {
	mailReplyCmd.Flags().StringVar(&mailReplyBody, "body", "", "Reply body")
	mailReplyCmd.Flags().BoolVar(&mailReplyAll, "all", false, "Reply to all recipients")
	mailReplyCmd.Flags().BoolVar(&mailReplyStdin, "stdin", false, "Read body from stdin")

	mailCmd.AddCommand(mailReplyCmd)
}

func runMailReply(cmd *cobra.Command, args []string) error {
	messageID := args[0]

	// Get body content
	body := mailReplyBody

	if mailReplyStdin {
		var err error
		body, err = readBodyFromStdin()
		if err != nil {
			return fmt.Errorf("error reading stdin: %w", err)
		}
	} else if body == "" && !IsNoInput() {
		Infof("Enter reply (Ctrl+D to finish):")
		var err error
		body, err = readBodyFromStdin()
		if err != nil {
			return fmt.Errorf("error reading stdin: %w", err)
		}
	}

	if body == "" {
		return fmt.Errorf("reply body is required")
	}

	client, ctx, err := newClientFromFlag(cmd)
	if err != nil {
		return fmt.Errorf("resolve account: %w", err)
	}

	if mailReplyAll {
		if err := client.ReplyAllToMessage(ctx, messageID, body); err != nil {
			return fmt.Errorf("failed to reply: %w", err)
		}
	} else {
		if err := client.ReplyToMessage(ctx, messageID, body); err != nil {
			return fmt.Errorf("failed to reply: %w", err)
		}
	}

	Infof("Reply sent successfully")
	return nil
}
