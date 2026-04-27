package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/spf13/cobra"
)

var (
	calAvailEmails []string
	calAvailStart  string
	calAvailEnd    string
)

var calendarAvailabilityCmd = &cobra.Command{
	Use:   "availability",
	Short: "Check free/busy availability for users",
	Long: `Check free/busy availability for one or more users.

Returns availability information for the specified time range.`,
	RunE: runCalendarAvailability,
}

func init() {
	calendarAvailabilityCmd.Flags().StringSliceVar(&calAvailEmails, "emails", nil, "Email addresses to check (required)")
	calendarAvailabilityCmd.Flags().StringVar(&calAvailStart, "start", "", "Start time (required)")
	calendarAvailabilityCmd.Flags().StringVar(&calAvailEnd, "end", "", "End time (required)")

	calendarAvailabilityCmd.MarkFlagRequired("emails")
	calendarAvailabilityCmd.MarkFlagRequired("start")
	calendarAvailabilityCmd.MarkFlagRequired("end")

	calendarCmd.AddCommand(calendarAvailabilityCmd)
}

func runCalendarAvailability(cmd *cobra.Command, args []string) error {
	if len(calAvailEmails) == 0 {
		return fmt.Errorf("--emails is required")
	}

	startTime, err := parseDateTime(calAvailStart)
	if err != nil {
		return fmt.Errorf("invalid start time: %w", err)
	}

	endTime, err := parseDateTime(calAvailEnd)
	if err != nil {
		return fmt.Errorf("invalid end time: %w", err)
	}

	account, err := auth.ResolveAccount(GetAccountFlag())
	if err != nil {
		return fmt.Errorf("resolve account: %w", err)
	}

	client := graph.NewClient(account)
	ctx := context.Background()

	schedules, err := client.GetSchedule(ctx, calAvailEmails, startTime, endTime)
	if err != nil {
		return fmt.Errorf("failed to get availability: %w", err)
	}

	format := GetOutputFormat()
	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(schedules)
	}

	// Table format
	for _, schedule := range schedules {
		fmt.Printf("=== %s ===\n", schedule.ScheduleID)

		if schedule.Error != nil {
			fmt.Printf("  Error: %s\n\n", schedule.Error.Message)
			continue
		}

		// Availability view is a string of status codes (0=free, 1=tentative, 2=busy, 3=oof, 4=working elsewhere)
		if schedule.AvailabilityView != "" {
			fmt.Printf("  Availability: %s\n", formatAvailabilityView(schedule.AvailabilityView))
		}

		if len(schedule.ScheduleItems) == 0 {
			fmt.Println("  No scheduled items in this range")
		} else {
			fmt.Println("  Schedule:")
			for _, item := range schedule.ScheduleItems {
				startStr := ""
				endStr := ""
				if item.Start != nil {
					st, _ := item.Start.ToTime()
					startStr = st.Local().Format("15:04")
				}
				if item.End != nil {
					et, _ := item.End.ToTime()
					endStr = et.Local().Format("15:04")
				}

				subject := item.Subject
				if item.IsPrivate {
					subject = "(Private)"
				}
				if subject == "" {
					subject = "(No subject)"
				}

				fmt.Printf("    %s - %s [%s] %s\n", startStr, endStr, item.Status, subject)
			}
		}
		fmt.Println()
	}

	return nil
}

func formatAvailabilityView(view string) string {
	// Convert status codes to readable format
	var parts []string
	statusMap := map[rune]string{
		'0': "free",
		'1': "tentative",
		'2': "busy",
		'3': "oof",
		'4': "working-elsewhere",
	}

	for _, r := range view {
		if s, ok := statusMap[r]; ok {
			parts = append(parts, s)
		} else {
			parts = append(parts, string(r))
		}
	}

	// Group consecutive identical statuses
	if len(parts) == 0 {
		return view
	}

	var result []string
	current := parts[0]
	count := 1

	for i := 1; i < len(parts); i++ {
		if parts[i] == current {
			count++
		} else {
			result = append(result, fmt.Sprintf("%dx%s", count, current))
			current = parts[i]
			count = 1
		}
	}
	result = append(result, fmt.Sprintf("%dx%s", count, current))

	return strings.Join(result, ", ")
}
