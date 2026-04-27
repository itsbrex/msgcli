package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/spf13/cobra"
)

var (
	calendarListStart string
	calendarListEnd   string
	calendarListLimit int
	calendarListID    string
)

var calendarListCmd = &cobra.Command{
	Use:   "list",
	Short: "List calendar events",
	Long: `List calendar events within a date range.

Date format: YYYY-MM-DD or YYYY-MM-DDTHH:MM:SS
Defaults to showing events for the next 7 days.`,
	RunE: runCalendarList,
}

func init() {
	calendarListCmd.Flags().StringVar(&calendarListStart, "start", "", "Start date (default: now)")
	calendarListCmd.Flags().StringVar(&calendarListEnd, "end", "", "End date (default: 7 days from start)")
	calendarListCmd.Flags().IntVarP(&calendarListLimit, "limit", "l", 50, "Maximum number of events")
	calendarListCmd.Flags().StringVar(&calendarListID, "calendar", "", "Calendar ID (default: primary)")
	calendarCmd.AddCommand(calendarListCmd)
}

func runCalendarList(cmd *cobra.Command, args []string) error {
	account, err := auth.ResolveAccount(GetAccountFlag())
	if err != nil {
		return fmt.Errorf("resolve account: %w", err)
	}

	// Parse date range
	var startTime, endTime *time.Time

	if calendarListStart != "" {
		t, err := parseDateTime(calendarListStart)
		if err != nil {
			return fmt.Errorf("invalid start date: %w", err)
		}
		startTime = &t
	} else {
		now := time.Now()
		startTime = &now
	}

	if calendarListEnd != "" {
		t, err := parseDateTime(calendarListEnd)
		if err != nil {
			return fmt.Errorf("invalid end date: %w", err)
		}
		endTime = &t
	} else {
		end := startTime.AddDate(0, 0, 7)
		endTime = &end
	}

	client := graph.NewClient(account)
	ctx := context.Background()

	params := &graph.QueryParams{
		Top:     calendarListLimit,
		OrderBy: "start/dateTime",
		Select:  []string{"id", "subject", "start", "end", "location", "isAllDay", "showAs", "organizer", "responseStatus"},
	}

	result, err := client.ListEvents(ctx, calendarListID, startTime, endTime, params)
	if err != nil {
		return fmt.Errorf("list events: %w", err)
	}

	format := GetOutputFormat()
	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result.Value)
	}

	// Table format
	if len(result.Value) == 0 {
		Infof("No events found")
		return nil
	}

	for _, event := range result.Value {
		startStr := "All day"
		endStr := ""
		if !event.IsAllDay && event.Start != nil {
			st, _ := event.Start.ToTime()
			startStr = st.Local().Format("Mon Jan 02 15:04")
			if event.End != nil {
				et, _ := event.End.ToTime()
				endStr = " - " + et.Local().Format("15:04")
			}
		} else if event.Start != nil {
			st, _ := event.Start.ToTime()
			startStr = st.Local().Format("Mon Jan 02") + " (all day)"
		}

		location := ""
		if event.Location != nil && event.Location.DisplayName != "" {
			location = " @ " + event.Location.DisplayName
		}

		status := ""
		if event.ResponseStatus != nil {
			switch event.ResponseStatus.Response {
			case "accepted":
				status = " ✓"
			case "tentativelyAccepted":
				status = " ?"
			case "declined":
				status = " ✗"
			}
		}

		showAs := ""
		switch event.ShowAs {
		case "busy":
			showAs = " [busy]"
		case "tentative":
			showAs = " [tentative]"
		case "oof":
			showAs = " [OOF]"
		case "free":
			showAs = " [free]"
		}

		fmt.Printf("%s%s%s%s%s\n", startStr, endStr, showAs, status, location)
		fmt.Printf("  %s\n", event.Subject)
		fmt.Printf("  ID: %s\n", event.ID)
		fmt.Println()
	}

	return nil
}

func parseDateTime(s string) (time.Time, error) {
	// Try various formats
	formats := []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.ParseInLocation(format, s, time.Local); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("could not parse date: %s (use YYYY-MM-DD or YYYY-MM-DDTHH:MM)", s)
}
