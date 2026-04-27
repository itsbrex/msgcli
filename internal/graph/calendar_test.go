package graph

import (
	"testing"
	"time"
)

func TestNewDateTimeZone(t *testing.T) {
	utc := time.Date(2026, 4, 27, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		t        time.Time
		tz       string
		wantTime string
		wantTZ   string
	}{
		{
			name:     "explicit UTC",
			t:        utc,
			tz:       "UTC",
			wantTime: "2026-04-27T10:30:00",
			wantTZ:   "UTC",
		},
		{
			name:     "named IANA timezone",
			t:        utc,
			tz:       "America/New_York",
			wantTime: "2026-04-27T10:30:00",
			wantTZ:   "America/New_York",
		},
		{
			name:     "empty timezone defaults to UTC",
			t:        utc,
			tz:       "",
			wantTime: "2026-04-27T10:30:00",
			wantTZ:   "UTC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewDateTimeZone(tt.t, tt.tz)
			if got == nil {
				t.Fatal("got nil DateTimeZone")
			}
			if got.DateTime != tt.wantTime {
				t.Errorf("DateTime = %q, want %q", got.DateTime, tt.wantTime)
			}
			if got.TimeZone != tt.wantTZ {
				t.Errorf("TimeZone = %q, want %q", got.TimeZone, tt.wantTZ)
			}
		})
	}
}

func TestDateTimeZone_ToTime(t *testing.T) {
	tests := []struct {
		name    string
		dt      DateTimeZone
		wantErr bool
		check   func(t *testing.T, got time.Time)
	}{
		{
			name: "UTC parses correctly",
			dt:   DateTimeZone{DateTime: "2026-04-27T10:30:00", TimeZone: "UTC"},
			check: func(t *testing.T, got time.Time) {
				want := time.Date(2026, 4, 27, 10, 30, 0, 0, time.UTC)
				if !got.Equal(want) {
					t.Errorf("got %v, want %v", got, want)
				}
			},
		},
		{
			name: "named IANA timezone preserves wall clock",
			dt:   DateTimeZone{DateTime: "2026-04-27T10:30:00", TimeZone: "America/New_York"},
			check: func(t *testing.T, got time.Time) {
				if got.Hour() != 10 || got.Minute() != 30 {
					t.Errorf("wall clock = %02d:%02d, want 10:30", got.Hour(), got.Minute())
				}
				if name, _ := got.Zone(); name != "EDT" && name != "EST" {
					t.Errorf("zone = %q, want EDT or EST", name)
				}
			},
		},
		{
			name: "unknown timezone falls back to UTC",
			dt:   DateTimeZone{DateTime: "2026-04-27T10:30:00", TimeZone: "Not/A_Real_Zone"},
			check: func(t *testing.T, got time.Time) {
				want := time.Date(2026, 4, 27, 10, 30, 0, 0, time.UTC)
				if !got.Equal(want) {
					t.Errorf("got %v, want %v (UTC fallback)", got, want)
				}
			},
		},
		{
			name:    "malformed datetime returns error",
			dt:      DateTimeZone{DateTime: "not-a-date", TimeZone: "UTC"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.dt.ToTime()
			if (err != nil) != tt.wantErr {
				t.Fatalf("ToTime() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}
