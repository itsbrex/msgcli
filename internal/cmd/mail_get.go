package cmd

import (
	"fmt"
	"strings"

	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/skylarbpayne/msgcli/internal/text"
	"github.com/spf13/cobra"
)

var mailGetCmd = &cobra.Command{
	Use:   "get <message-id>",
	Short: "Get a specific email message",
	Args:  cobra.ExactArgs(1),
	RunE:  runMailGet,
}

func init() {
	mailCmd.AddCommand(mailGetCmd)
}

func runMailGet(cmd *cobra.Command, args []string) error {
	messageID := args[0]

	client, ctx, err := newClientFromFlag()
	if err != nil {
		return fmt.Errorf("resolve account: %w", err)
	}

	msg, err := client.GetMessage(ctx, messageID)
	if err != nil {
		return fmt.Errorf("get message: %w", err)
	}

	format := GetOutputFormat()
	if format == "json" {
		return writeJSON(msg)
	}

	// Table/text format
	fmt.Printf("ID:      %s\n", msg.ID)
	fmt.Printf("Subject: %s\n", msg.Subject)

	if msg.From != nil {
		from := msg.From.EmailAddress.Address
		if msg.From.EmailAddress.Name != "" {
			from = fmt.Sprintf("%s <%s>", msg.From.EmailAddress.Name, msg.From.EmailAddress.Address)
		}
		fmt.Printf("From:    %s\n", from)
	}

	if len(msg.ToRecipients) > 0 {
		to := formatRecipients(msg.ToRecipients)
		fmt.Printf("To:      %s\n", to)
	}

	if len(msg.CcRecipients) > 0 {
		cc := formatRecipients(msg.CcRecipients)
		fmt.Printf("Cc:      %s\n", cc)
	}

	fmt.Printf("Date:    %s\n", msg.ReceivedDateTime.Local().Format("Mon, 02 Jan 2006 15:04:05 MST"))
	fmt.Printf("Read:    %v\n", msg.IsRead)

	if msg.HasAttachments {
		fmt.Println("Attach:  Yes")
	}

	fmt.Println()
	fmt.Println("--- Body ---")
	if msg.Body != nil {
		content := msg.Body.Content
		if msg.Body.ContentType == "html" {
			// Basic HTML stripping for display
			content = text.StripHTML(content)
		}
		fmt.Println(content)
	} else {
		fmt.Println(msg.BodyPreview)
	}

	return nil
}

func formatRecipients(recipients []graph.Recipient) string {
	addrs := make([]string, len(recipients))
	for i, r := range recipients {
		if r.EmailAddress.Name != "" {
			addrs[i] = fmt.Sprintf("%s <%s>", r.EmailAddress.Name, r.EmailAddress.Address)
		} else {
			addrs[i] = r.EmailAddress.Address
		}
	}
	return strings.Join(addrs, ", ")
}
