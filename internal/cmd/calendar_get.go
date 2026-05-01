package cmd

import (
	"fmt"
	"strings"

	"github.com/skylarbpayne/msgcli/internal/text"
	"github.com/spf13/cobra"
)

var calendarGetCmd = &cobra.Command{
	Use:   "get <event-id>",
	Short: "Get a specific calendar event",
	Args:  cobra.ExactArgs(1),
	RunE:  runCalendarGet,
}

func init() {
	calendarCmd.AddCommand(calendarGetCmd)
}

func runCalendarGet(cmd *cobra.Command, args []string) error {
	eventID := args[0]

	client, ctx, err := newClientFromFlag(cmd)
	if err != nil {
		return fmt.Errorf("resolve account: %w", err)
	}

	event, err := client.GetEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("get event: %w", err)
	}

	format := GetOutputFormat()
	if format == "json" {
		return writeJSON(event)
	}

	// Text format
	fmt.Printf("ID:       %s\n", event.ID)
	fmt.Printf("Subject:  %s\n", event.Subject)

	if event.Start != nil {
		st, _ := event.Start.ToTime()
		if event.IsAllDay {
			fmt.Printf("Start:    %s (all day)\n", st.Local().Format("Mon, 02 Jan 2006"))
		} else {
			fmt.Printf("Start:    %s\n", st.Local().Format("Mon, 02 Jan 2006 15:04 MST"))
		}
	}

	if event.End != nil {
		et, _ := event.End.ToTime()
		if !event.IsAllDay {
			fmt.Printf("End:      %s\n", et.Local().Format("Mon, 02 Jan 2006 15:04 MST"))
		}
	}

	if event.Location != nil && event.Location.DisplayName != "" {
		fmt.Printf("Location: %s\n", event.Location.DisplayName)
	}

	if event.OnlineMeetingURL != "" {
		fmt.Printf("Meeting:  %s\n", event.OnlineMeetingURL)
	}

	if event.Organizer != nil {
		org := event.Organizer.EmailAddress.Address
		if event.Organizer.EmailAddress.Name != "" {
			org = fmt.Sprintf("%s <%s>", event.Organizer.EmailAddress.Name, event.Organizer.EmailAddress.Address)
		}
		fmt.Printf("Organizer: %s\n", org)
	}

	if len(event.Attendees) > 0 {
		fmt.Println("Attendees:")
		for _, att := range event.Attendees {
			name := att.EmailAddress.Address
			if att.EmailAddress.Name != "" {
				name = fmt.Sprintf("%s <%s>", att.EmailAddress.Name, att.EmailAddress.Address)
			}
			status := ""
			if att.Status != nil {
				status = " (" + att.Status.Response + ")"
			}
			fmt.Printf("  - %s [%s]%s\n", name, att.Type, status)
		}
	}

	fmt.Printf("Show as:  %s\n", event.ShowAs)

	if event.ResponseStatus != nil {
		fmt.Printf("Response: %s\n", event.ResponseStatus.Response)
	}

	if event.Body != nil && event.Body.Content != "" {
		fmt.Println()
		fmt.Println("--- Description ---")
		content := event.Body.Content
		if event.Body.ContentType == "html" {
			content = text.StripHTML(content)
		}
		fmt.Println(strings.TrimSpace(content))
	}

	return nil
}
