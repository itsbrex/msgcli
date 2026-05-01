package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/spf13/cobra"
)

var (
	calCreateSubject   string
	calCreateStart     string
	calCreateEnd       string
	calCreateLocation  string
	calCreateBody      string
	calCreateAttendees []string
	calCreateAllDay    bool
	calCreateTimezone  string
)

var calendarCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new calendar event",
	Long: `Create a new calendar event.

Date/time format: YYYY-MM-DDTHH:MM (e.g., 2024-01-15T14:00)
For all-day events, use YYYY-MM-DD format.`,
	RunE: runCalendarCreate,
}

func init() {
	calendarCreateCmd.Flags().StringVar(&calCreateSubject, "subject", "", "Event subject/title (required)")
	calendarCreateCmd.Flags().StringVar(&calCreateStart, "start", "", "Start time (required)")
	calendarCreateCmd.Flags().StringVar(&calCreateEnd, "end", "", "End time (required for non-all-day events)")
	calendarCreateCmd.Flags().StringVar(&calCreateLocation, "location", "", "Event location")
	calendarCreateCmd.Flags().StringVar(&calCreateBody, "body", "", "Event description")
	calendarCreateCmd.Flags().StringSliceVar(&calCreateAttendees, "attendees", nil, "Attendee email addresses")
	calendarCreateCmd.Flags().BoolVar(&calCreateAllDay, "all-day", false, "Create an all-day event")
	calendarCreateCmd.Flags().StringVar(&calCreateTimezone, "timezone", "", "Timezone (default: local)")

	calendarCreateCmd.MarkFlagRequired("subject")
	calendarCreateCmd.MarkFlagRequired("start")

	calendarCmd.AddCommand(calendarCreateCmd)
}

func runCalendarCreate(cmd *cobra.Command, args []string) error {
	if calCreateSubject == "" {
		return fmt.Errorf("--subject is required")
	}
	if calCreateStart == "" {
		return fmt.Errorf("--start is required")
	}
	if !calCreateAllDay && calCreateEnd == "" {
		return fmt.Errorf("--end is required (or use --all-day)")
	}

	// Parse times
	startTime, err := parseDateTime(calCreateStart)
	if err != nil {
		return fmt.Errorf("invalid start time: %w", err)
	}

	var endTime time.Time
	if calCreateAllDay {
		// For all-day events, end is the next day
		endTime = startTime.AddDate(0, 0, 1)
	} else {
		endTime, err = parseDateTime(calCreateEnd)
		if err != nil {
			return fmt.Errorf("invalid end time: %w", err)
		}
	}

	// Determine timezone
	tz := calCreateTimezone
	if tz == "" {
		tz = time.Local.String()
		// Handle cases where Local doesn't have a name
		if tz == "Local" {
			_, offset := time.Now().Zone()
			if offset == 0 {
				tz = "UTC"
			} else {
				// Try to get the actual timezone name
				tz = "UTC"
			}
		}
	}

	client, ctx, err := newClientFromFlag(cmd)
	if err != nil {
		return fmt.Errorf("resolve account: %w", err)
	}

	event := &graph.Event{
		Subject:  calCreateSubject,
		Start:    graph.NewDateTimeZone(startTime, tz),
		End:      graph.NewDateTimeZone(endTime, tz),
		IsAllDay: calCreateAllDay,
	}

	if calCreateLocation != "" {
		event.Location = &graph.Location{DisplayName: calCreateLocation}
	}

	if calCreateBody != "" {
		event.Body = &graph.ItemBody{
			ContentType: "text",
			Content:     calCreateBody,
		}
	}

	if len(calCreateAttendees) > 0 {
		event.Attendees = make([]graph.Attendee, len(calCreateAttendees))
		for i, email := range calCreateAttendees {
			event.Attendees[i] = graph.Attendee{
				EmailAddress: graph.EmailAddress{Address: strings.TrimSpace(email)},
				Type:         "required",
			}
		}
	}

	result, err := client.CreateEvent(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to create event: %w", err)
	}

	format := GetOutputFormat()
	if format == "json" {
		return writeJSON(result)
	}

	Infof("Event created successfully")
	Infof("ID: %s", result.ID)
	if result.WebLink != "" {
		Infof("Link: %s", result.WebLink)
	}

	return nil
}
