package graph

import (
	"context"
	"fmt"
	"time"
)

// Event represents an Outlook calendar event
type Event struct {
	ID                    string          `json:"id,omitempty"`
	Subject               string          `json:"subject"`
	Body                  *ItemBody       `json:"body,omitempty"`
	BodyPreview           string          `json:"bodyPreview,omitempty"`
	Start                 *DateTimeZone   `json:"start"`
	End                   *DateTimeZone   `json:"end"`
	Location              *Location       `json:"location,omitempty"`
	Attendees             []Attendee      `json:"attendees,omitempty"`
	Organizer             *Recipient      `json:"organizer,omitempty"`
	IsAllDay              bool            `json:"isAllDay"`
	IsCancelled           bool            `json:"isCancelled"`
	IsOrganizer           bool            `json:"isOrganizer"`
	ResponseStatus        *ResponseStatus `json:"responseStatus,omitempty"`
	ShowAs                string          `json:"showAs,omitempty"` // free, tentative, busy, oof, workingElsewhere, unknown
	Importance            string          `json:"importance,omitempty"`
	Sensitivity           string          `json:"sensitivity,omitempty"`
	Recurrence            *Recurrence     `json:"recurrence,omitempty"`
	WebLink               string          `json:"webLink,omitempty"`
	OnlineMeetingURL      string          `json:"onlineMeetingUrl,omitempty"`
	IsOnlineMeeting       bool            `json:"isOnlineMeeting"`
	OnlineMeetingProvider string          `json:"onlineMeetingProvider,omitempty"`
	CreatedDateTime       time.Time       `json:"createdDateTime,omitempty"`
	LastModifiedDateTime  time.Time       `json:"lastModifiedDateTime,omitempty"`
}

// DateTimeZone represents a date/time with timezone
type DateTimeZone struct {
	DateTime string `json:"dateTime"` // ISO 8601 format without offset
	TimeZone string `json:"timeZone"` // IANA timezone name
}

// Location represents an event location
type Location struct {
	DisplayName  string   `json:"displayName,omitempty"`
	LocationType string   `json:"locationType,omitempty"`
	Address      *Address `json:"address,omitempty"`
}

// Address represents a physical address
type Address struct {
	Street          string `json:"street,omitempty"`
	City            string `json:"city,omitempty"`
	State           string `json:"state,omitempty"`
	CountryOrRegion string `json:"countryOrRegion,omitempty"`
	PostalCode      string `json:"postalCode,omitempty"`
}

// Attendee represents an event attendee
type Attendee struct {
	EmailAddress EmailAddress    `json:"emailAddress"`
	Type         string          `json:"type"` // required, optional, resource
	Status       *ResponseStatus `json:"status,omitempty"`
}

// ResponseStatus represents an attendee's response
type ResponseStatus struct {
	Response string    `json:"response"` // none, organizer, tentativelyAccepted, accepted, declined, notResponded
	Time     time.Time `json:"time,omitempty"`
}

// Recurrence represents event recurrence pattern
type Recurrence struct {
	Pattern *RecurrencePattern `json:"pattern,omitempty"`
	Range   *RecurrenceRange   `json:"range,omitempty"`
}

// RecurrencePattern represents how the event recurs
type RecurrencePattern struct {
	Type           string   `json:"type"` // daily, weekly, absoluteMonthly, relativeMonthly, absoluteYearly, relativeYearly
	Interval       int      `json:"interval"`
	DaysOfWeek     []string `json:"daysOfWeek,omitempty"`
	DayOfMonth     int      `json:"dayOfMonth,omitempty"`
	Month          int      `json:"month,omitempty"`
	FirstDayOfWeek string   `json:"firstDayOfWeek,omitempty"`
}

// RecurrenceRange represents when the recurrence ends
type RecurrenceRange struct {
	Type                string `json:"type"` // endDate, noEnd, numbered
	StartDate           string `json:"startDate"`
	EndDate             string `json:"endDate,omitempty"`
	NumberOfOccurrences int    `json:"numberOfOccurrences,omitempty"`
}

// Calendar represents an Outlook calendar
type Calendar struct {
	ID                  string        `json:"id"`
	Name                string        `json:"name"`
	Color               string        `json:"color,omitempty"`
	IsDefaultCalendar   bool          `json:"isDefaultCalendar"`
	CanViewPrivateItems bool          `json:"canViewPrivateItems"`
	CanEdit             bool          `json:"canEdit"`
	CanShare            bool          `json:"canShare"`
	Owner               *EmailAddress `json:"owner,omitempty"`
}

// ScheduleInfo represents free/busy information for a user
type ScheduleInfo struct {
	ScheduleID       string         `json:"scheduleId"`
	AvailabilityView string         `json:"availabilityView"`
	ScheduleItems    []ScheduleItem `json:"scheduleItems"`
	WorkingHours     *WorkingHours  `json:"workingHours,omitempty"`
	Error            *ScheduleError `json:"error,omitempty"`
}

// ScheduleItem represents a time slot in a schedule
type ScheduleItem struct {
	Start     *DateTimeZone `json:"start"`
	End       *DateTimeZone `json:"end"`
	Status    string        `json:"status"` // free, tentative, busy, oof, workingElsewhere, unknown
	Subject   string        `json:"subject,omitempty"`
	Location  string        `json:"location,omitempty"`
	IsPrivate bool          `json:"isPrivate"`
}

// WorkingHours represents user's working hours
type WorkingHours struct {
	DaysOfWeek []string      `json:"daysOfWeek"`
	StartTime  string        `json:"startTime"`
	EndTime    string        `json:"endTime"`
	TimeZone   *TimeZoneBase `json:"timeZone"`
}

// TimeZoneBase represents a timezone
type TimeZoneBase struct {
	Name string `json:"name"`
}

// ScheduleError represents an error getting schedule
type ScheduleError struct {
	Message      string `json:"message"`
	ResponseCode string `json:"responseCode"`
}

// ListEvents lists calendar events
func (c *Client) ListEvents(ctx context.Context, calendarID string, startTime, endTime *time.Time, params *QueryParams) (*ListResponse[Event], error) {
	path := "/me/calendar/events"
	if calendarID != "" {
		path = fmt.Sprintf("/me/calendars/%s/events", calendarID)
	}

	// Build filter for date range
	if startTime != nil || endTime != nil {
		filter := ""
		if startTime != nil {
			filter = fmt.Sprintf("start/dateTime ge '%s'", startTime.Format(time.RFC3339))
		}
		if endTime != nil {
			if filter != "" {
				filter += " and "
			}
			filter += fmt.Sprintf("end/dateTime le '%s'", endTime.Format(time.RFC3339))
		}
		if params == nil {
			params = &QueryParams{}
		}
		if params.Filter != "" {
			params.Filter = params.Filter + " and " + filter
		} else {
			params.Filter = filter
		}
	}

	if params != nil {
		path += params.ToQuery()
	}

	var result ListResponse[Event]
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetEvent retrieves a single event by ID
func (c *Client) GetEvent(ctx context.Context, eventID string) (*Event, error) {
	path := fmt.Sprintf("/me/events/%s", eventID)

	var result Event
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// CreateEvent creates a new calendar event
func (c *Client) CreateEvent(ctx context.Context, event *Event) (*Event, error) {
	var result Event
	if err := c.Post(ctx, "/me/events", event, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// UpdateEvent updates an existing calendar event
func (c *Client) UpdateEvent(ctx context.Context, eventID string, updates map[string]interface{}) (*Event, error) {
	path := fmt.Sprintf("/me/events/%s", eventID)

	var result Event
	if err := c.Patch(ctx, path, updates, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// DeleteEvent deletes a calendar event
func (c *Client) DeleteEvent(ctx context.Context, eventID string) error {
	path := fmt.Sprintf("/me/events/%s", eventID)
	return c.Delete(ctx, path)
}

// AcceptEvent accepts a meeting invitation
func (c *Client) AcceptEvent(ctx context.Context, eventID string, comment string, sendResponse bool) error {
	path := fmt.Sprintf("/me/events/%s/accept", eventID)
	return c.Post(ctx, path, respondBody(comment, sendResponse), nil)
}

// DeclineEvent declines a meeting invitation
func (c *Client) DeclineEvent(ctx context.Context, eventID string, comment string, sendResponse bool) error {
	path := fmt.Sprintf("/me/events/%s/decline", eventID)
	return c.Post(ctx, path, respondBody(comment, sendResponse), nil)
}

// TentativelyAcceptEvent tentatively accepts a meeting
func (c *Client) TentativelyAcceptEvent(ctx context.Context, eventID string, comment string, sendResponse bool) error {
	path := fmt.Sprintf("/me/events/%s/tentativelyAccept", eventID)
	return c.Post(ctx, path, respondBody(comment, sendResponse), nil)
}

// respondBody builds the Graph payload for accept/decline/tentativelyAccept.
// Graph rejects a non-null comment when sendResponse is false, so omit the
// comment field entirely when empty.
func respondBody(comment string, sendResponse bool) map[string]interface{} {
	body := map[string]interface{}{"sendResponse": sendResponse}
	if comment != "" {
		body["comment"] = comment
	}
	return body
}

// CancelEvent cancels an event (only for organizer)
func (c *Client) CancelEvent(ctx context.Context, eventID string, comment string) error {
	path := fmt.Sprintf("/me/events/%s/cancel", eventID)
	body := map[string]string{"comment": comment}
	return c.Post(ctx, path, body, nil)
}

// GetSchedule gets free/busy information for users
func (c *Client) GetSchedule(ctx context.Context, emails []string, startTime, endTime time.Time) ([]ScheduleInfo, error) {
	body := map[string]interface{}{
		"schedules":                emails,
		"startTime":                DateTimeZone{DateTime: startTime.Format("2006-01-02T15:04:05"), TimeZone: "UTC"},
		"endTime":                  DateTimeZone{DateTime: endTime.Format("2006-01-02T15:04:05"), TimeZone: "UTC"},
		"availabilityViewInterval": 30,
	}

	var result struct {
		Value []ScheduleInfo `json:"value"`
	}
	if err := c.Post(ctx, "/me/calendar/getSchedule", body, &result); err != nil {
		return nil, err
	}

	return result.Value, nil
}

// ListCalendars lists all calendars
func (c *Client) ListCalendars(ctx context.Context) (*ListResponse[Calendar], error) {
	var result ListResponse[Calendar]
	if err := c.Get(ctx, "/me/calendars", &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// NewDateTimeZone creates a DateTimeZone from a time.Time
func NewDateTimeZone(t time.Time, tz string) *DateTimeZone {
	if tz == "" {
		tz = "UTC"
	}
	return &DateTimeZone{
		DateTime: t.Format("2006-01-02T15:04:05"),
		TimeZone: tz,
	}
}

// ToTime converts a DateTimeZone to time.Time
func (d *DateTimeZone) ToTime() (time.Time, error) {
	loc, err := time.LoadLocation(d.TimeZone)
	if err != nil {
		loc = time.UTC
	}
	return time.ParseInLocation("2006-01-02T15:04:05", d.DateTime, loc)
}
