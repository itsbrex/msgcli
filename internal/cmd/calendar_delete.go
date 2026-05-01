package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	calDeleteForce   bool
	calDeleteCancel  bool
	calDeleteComment string
)

var calendarDeleteCmd = &cobra.Command{
	Use:   "delete <event-id>",
	Short: "Delete a calendar event",
	Long: `Delete a calendar event.

If you're the organizer, use --cancel to notify attendees.`,
	Args: cobra.ExactArgs(1),
	RunE: runCalendarDelete,
}

func init() {
	calendarDeleteCmd.Flags().BoolVarP(&calDeleteForce, "force", "f", false, "Skip confirmation prompt")
	calendarDeleteCmd.Flags().BoolVar(&calDeleteCancel, "cancel", false, "Cancel and notify attendees (for organizer)")
	calendarDeleteCmd.Flags().StringVar(&calDeleteComment, "comment", "", "Cancellation message (with --cancel)")
	calendarCmd.AddCommand(calendarDeleteCmd)
}

func runCalendarDelete(cmd *cobra.Command, args []string) error {
	eventID := args[0]

	client, ctx, err := newClientFromFlag()
	if err != nil {
		return fmt.Errorf("resolve account: %w", err)
	}

	if !calDeleteForce && !IsNoInput() {
		event, err := client.GetEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("failed to get event: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Delete event: \"%s\"? [y/N]: ", event.Subject)
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			Infof("Cancelled")
			return nil
		}
	}

	if calDeleteCancel {
		if err := client.CancelEvent(ctx, eventID, calDeleteComment); err != nil {
			return fmt.Errorf("failed to cancel event: %w", err)
		}
		Infof("Event cancelled and attendees notified")
	} else {
		if err := client.DeleteEvent(ctx, eventID); err != nil {
			return fmt.Errorf("failed to delete event: %w", err)
		}
		Infof("Event deleted")
	}

	return nil
}
