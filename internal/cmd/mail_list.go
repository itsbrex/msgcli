package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/spf13/cobra"
)

var (
	mailListFolder string
	mailListLimit  int
	mailListQuery  string
	mailListFullID bool
)

var mailListCmd = &cobra.Command{
	Use:   "list",
	Short: "List email messages",
	Long: `List email messages in a folder.

Well-known folder names: inbox, drafts, sentitems, deleteditems, archive, junkemail`,
	RunE: runMailList,
}

func init() {
	mailListCmd.Flags().StringVarP(&mailListFolder, "folder", "f", "inbox", "Folder to list (inbox, drafts, sentitems, etc.)")
	mailListCmd.Flags().IntVarP(&mailListLimit, "limit", "l", 25, "Maximum number of messages to return")
	mailListCmd.Flags().StringVarP(&mailListQuery, "query", "q", "", "Search query (KQL syntax)")
	mailListCmd.Flags().BoolVar(&mailListFullID, "full-id", false, "Show full message IDs in table output")
	mailCmd.AddCommand(mailListCmd)
}

func runMailList(cmd *cobra.Command, args []string) error {
	client, ctx, err := newClientFromFlag()
	if err != nil {
		return fmt.Errorf("resolve account: %w", err)
	}

	var result *graph.ListResponse[graph.Message]

	if mailListQuery != "" {
		result, err = client.SearchMessages(ctx, mailListQuery, mailListLimit)
	} else {
		params := &graph.QueryParams{
			Top:     mailListLimit,
			OrderBy: "receivedDateTime desc",
			Select:  []string{"id", "subject", "from", "receivedDateTime", "isRead", "hasAttachments", "bodyPreview"},
		}
		result, err = client.ListMessages(ctx, mailListFolder, params)
	}

	if err != nil {
		return fmt.Errorf("list messages: %w", err)
	}

	format := GetOutputFormat()
	if format == "json" {
		return writeJSON(result.Value)
	}

	// Table format
	if len(result.Value) == 0 {
		Infof("No messages found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "STATE\tFROM\tSUBJECT\tRECEIVED\tID")
	fmt.Fprintln(w, "-----\t----\t-------\t--------\t--")

	for _, msg := range result.Value {
		state := "R"
		if !msg.IsRead {
			state = "N"
		}
		if msg.HasAttachments {
			state += "+"
		}

		from := ""
		if msg.From != nil {
			from = msg.From.EmailAddress.Address
			if msg.From.EmailAddress.Name != "" {
				from = msg.From.EmailAddress.Name
			}
		}

		subject := truncateText(msg.Subject, 58)
		timeStr := msg.ReceivedDateTime.Local().Format("Jan 02 15:04")
		msgID := msg.ID
		if !mailListFullID {
			msgID = shortMessageID(msg.ID)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			state,
			truncateText(from, 24),
			subject,
			timeStr,
			msgID,
		)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush output: %w", err)
	}
	if !mailListFullID {
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "Tip: use --full-id to print complete message IDs")
	}

	return nil
}

func truncateText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func shortMessageID(s string) string {
	if len(s) <= 30 {
		return s
	}
	const headLen = 14
	const tailLen = 12
	return s[:headLen] + "..." + s[len(s)-tailLen:]
}
