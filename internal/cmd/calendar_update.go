package cmd

import (
	"fmt"
	"time"

	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/spf13/cobra"
)

var (
	calUpdateSubject  string
	calUpdateStart    string
	calUpdateEnd      string
	calUpdateLocation string
	calUpdateBody     string
	calUpdateTimezone string
)

var calendarUpdateCmd = &cobra.Command{
	Use:   "update <event-id>",
	Short: "Update a calendar event",
	Args:  cobra.ExactArgs(1),
	RunE:  runCalendarUpdate,
}

func init() {
	calendarUpdateCmd.Flags().StringVar(&calUpdateSubject, "subject", "", "New event subject")
	calendarUpdateCmd.Flags().StringVar(&calUpdateStart, "start", "", "New start time")
	calendarUpdateCmd.Flags().StringVar(&calUpdateEnd, "end", "", "New end time")
	calendarUpdateCmd.Flags().StringVar(&calUpdateLocation, "location", "", "New location")
	calendarUpdateCmd.Flags().StringVar(&calUpdateBody, "body", "", "New description")
	calendarUpdateCmd.Flags().StringVar(&calUpdateTimezone, "timezone", "", "Timezone for times")

	calendarCmd.AddCommand(calendarUpdateCmd)
}

func runCalendarUpdate(cmd *cobra.Command, args []string) error {
	eventID := args[0]

	updates := make(map[string]interface{})

	if calUpdateSubject != "" {
		updates["subject"] = calUpdateSubject
	}

	tz := calUpdateTimezone
	if tz == "" {
		tz = time.Local.String()
		if tz == "Local" {
			tz = "UTC"
		}
	}

	if calUpdateStart != "" {
		t, err := parseDateTime(calUpdateStart)
		if err != nil {
			return fmt.Errorf("invalid start time: %w", err)
		}
		updates["start"] = graph.NewDateTimeZone(t, tz)
	}

	if calUpdateEnd != "" {
		t, err := parseDateTime(calUpdateEnd)
		if err != nil {
			return fmt.Errorf("invalid end time: %w", err)
		}
		updates["end"] = graph.NewDateTimeZone(t, tz)
	}

	if calUpdateLocation != "" {
		updates["location"] = map[string]string{"displayName": calUpdateLocation}
	}

	if calUpdateBody != "" {
		updates["body"] = map[string]string{
			"contentType": "text",
			"content":     calUpdateBody,
		}
	}

	if len(updates) == 0 {
		return fmt.Errorf("no updates specified - use --subject, --start, --end, --location, or --body")
	}

	client, ctx, err := newClientFromFlag(cmd)
	if err != nil {
		return fmt.Errorf("resolve account: %w", err)
	}

	result, err := client.UpdateEvent(ctx, eventID, updates)
	if err != nil {
		return fmt.Errorf("failed to update event: %w", err)
	}

	format := GetOutputFormat()
	if format == "json" {
		return writeJSON(result)
	}

	Infof("Event updated successfully")
	return nil
}
