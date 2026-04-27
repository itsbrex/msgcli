package cmd

import (
	"context"
	"fmt"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/spf13/cobra"
)

var (
	calRespondResponse string
	calRespondComment  string
	calRespondSilent   bool
)

var calendarRespondCmd = &cobra.Command{
	Use:   "respond <event-id>",
	Short: "Respond to a meeting invitation",
	Long: `Respond to a meeting invitation with accept, decline, or tentative.

Examples:
  msgcli calendar respond EVENT_ID --response accept
  msgcli calendar respond EVENT_ID --response decline --comment "Can't make it"
  msgcli calendar respond EVENT_ID --response tentative --silent`,
	Args: cobra.ExactArgs(1),
	RunE: runCalendarRespond,
}

func init() {
	calendarRespondCmd.Flags().StringVarP(&calRespondResponse, "response", "r", "", "Response: accept, decline, or tentative (required)")
	calendarRespondCmd.Flags().StringVar(&calRespondComment, "comment", "", "Optional response comment")
	calendarRespondCmd.Flags().BoolVar(&calRespondSilent, "silent", false, "Don't send response to organizer")

	calendarRespondCmd.MarkFlagRequired("response")
	calendarCmd.AddCommand(calendarRespondCmd)
}

func runCalendarRespond(cmd *cobra.Command, args []string) error {
	eventID := args[0]

	account, err := auth.ResolveAccount(GetAccountFlag())
	if err != nil {
		return fmt.Errorf("resolve account: %w", err)
	}

	client := graph.NewClient(account)
	ctx := context.Background()

	sendResponse := !calRespondSilent

	switch calRespondResponse {
	case "accept":
		if err := client.AcceptEvent(ctx, eventID, calRespondComment, sendResponse); err != nil {
			return fmt.Errorf("failed to accept: %w", err)
		}
		Infof("Meeting accepted")

	case "decline":
		if err := client.DeclineEvent(ctx, eventID, calRespondComment, sendResponse); err != nil {
			return fmt.Errorf("failed to decline: %w", err)
		}
		Infof("Meeting declined")

	case "tentative":
		if err := client.TentativelyAcceptEvent(ctx, eventID, calRespondComment, sendResponse); err != nil {
			return fmt.Errorf("failed to respond: %w", err)
		}
		Infof("Meeting tentatively accepted")

	default:
		return fmt.Errorf("invalid response: %s (use accept, decline, or tentative)", calRespondResponse)
	}

	return nil
}
