package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	mailSendTo      []string
	mailSendCc      []string
	mailSendSubject string
	mailSendBody    string
	mailSendHTML    bool
	mailSendStdin   bool
)

var mailSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send an email message",
	Long: `Send an email message.

The body can be provided via --body flag, --stdin flag (reads from stdin),
or interactively if neither is specified.`,
	RunE: runMailSend,
}

func init() {
	mailSendCmd.Flags().StringSliceVar(&mailSendTo, "to", nil, "Recipient email addresses (required)")
	mailSendCmd.Flags().StringSliceVar(&mailSendCc, "cc", nil, "CC email addresses")
	mailSendCmd.Flags().StringVar(&mailSendSubject, "subject", "", "Email subject (required)")
	mailSendCmd.Flags().StringVar(&mailSendBody, "body", "", "Email body")
	mailSendCmd.Flags().BoolVar(&mailSendHTML, "html", false, "Treat body as HTML")
	mailSendCmd.Flags().BoolVar(&mailSendStdin, "stdin", false, "Read body from stdin")

	mailSendCmd.MarkFlagRequired("to")
	mailSendCmd.MarkFlagRequired("subject")

	mailCmd.AddCommand(mailSendCmd)
}

func runMailSend(cmd *cobra.Command, args []string) error {
	if len(mailSendTo) == 0 {
		return fmt.Errorf("at least one --to recipient is required")
	}

	if mailSendSubject == "" {
		return fmt.Errorf("--subject is required")
	}

	// Get body content
	body := mailSendBody

	if mailSendStdin {
		// Read from stdin
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
		// Interactive input
		Infof("Enter message body (Ctrl+D to finish):")
		var sb strings.Builder
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			sb.WriteString(scanner.Text())
			sb.WriteString("\n")
		}
		body = strings.TrimSuffix(sb.String(), "\n")
	}

	if body == "" {
		return fmt.Errorf("message body is required")
	}

	client, ctx, err := newClientFromFlag()
	if err != nil {
		return fmt.Errorf("resolve account: %w", err)
	}

	if err := client.SendMail(ctx, mailSendTo, mailSendCc, mailSendSubject, body, mailSendHTML); err != nil {
		return fmt.Errorf("failed to send mail: %w", err)
	}

	Infof("Message sent successfully")
	return nil
}
