package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

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
		var sb strings.Builder
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			sb.WriteString(scanner.Text())
			sb.WriteString("\n")
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error reading stdin: %w", err)
		}
		body = strings.TrimSuffix(sb.String(), "\n")
	} else if body == "" && !IsNoInput() {
		Infof("Enter reply (Ctrl+D to finish):")
		var sb strings.Builder
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			sb.WriteString(scanner.Text())
			sb.WriteString("\n")
		}
		body = strings.TrimSuffix(sb.String(), "\n")
	}

	if body == "" {
		return fmt.Errorf("reply body is required")
	}

	client, ctx, err := newClientFromFlag()
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
