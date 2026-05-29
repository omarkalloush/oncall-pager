package schedule

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetOnCallEmail(t *testing.T) {
	// Mock CSV data
	csvData := `Day,Start Time,End Time,Email
Monday,3:00 AM,6:00 PM,monday@example.com
Friday,3:00 AM,6:00 PM,friday@example.com
`

	// Start a local HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, csvData)
	}))
	defer server.Close()

	// Helper to create a fixed time in EST
	mockTime := func(day time.Weekday, hour, minute int) func() time.Time {
		return func() time.Time {
			loc, _ := time.LoadLocation("America/New_York")
			// Create a date in 2026 where May 29 = Friday. 
			// Offset the date to match the requested weekday.
			offset := int(day) - int(time.Friday) 
			return time.Date(2026, time.May, 29+offset, hour, minute, 0, 0, loc)
		}
	}

	tests := []struct {
		name          string
		mockTime      func() time.Time
		expectedEmail string
		expectedError string
	}{
		{
			name:          "Success - Friday inside slot",
			mockTime:      mockTime(time.Friday, 15, 0), // 3:00 PM
			expectedEmail: "friday@example.com",
			expectedError: "",
		},
		{
			name:          "Failure - Saturday (Weekend)",
			mockTime:      mockTime(time.Saturday, 12, 0),
			expectedEmail: "",
			expectedError: "today is Saturday, no one is on call",
		},
		{
			name:          "Failure - Out of hours (2 AM)",
			mockTime:      mockTime(time.Monday, 2, 0),
			expectedEmail: "",
			expectedError: "No one is on-call at the moment, we will reply to you as soon as possible!",
		},
		{
			name:          "Failure - Out of hours (7 PM)",
			mockTime:      mockTime(time.Monday, 19, 0),
			expectedEmail: "",
			expectedError: "No one is on-call at the moment, we will reply to you as soon as possible!",
		},
		{
			name:          "Failure - Tuesday (No slot in CSV)",
			mockTime:      mockTime(time.Tuesday, 12, 0),
			expectedEmail: "",
			expectedError: "No one is on-call at the moment, we will reply to you as soon as possible!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewProvider(server.URL)
			provider.TimeNow = tt.mockTime

			email, err := provider.GetOnCallEmail()

			if tt.expectedError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.expectedError)
				}
				if err.Error() != tt.expectedError {
					t.Errorf("expected error %q, got %q", tt.expectedError, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if email != tt.expectedEmail {
					t.Errorf("expected email %q, got %q", tt.expectedEmail, email)
				}
			}
		})
	}
}
