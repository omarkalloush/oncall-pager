package schedule

import (
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

type Provider struct {
	SheetURL string
	TimeNow  func() time.Time
}

func NewProvider(sheetURL string) *Provider {
	return &Provider{
		SheetURL: sheetURL,
		TimeNow:  time.Now,
	}
}

// GetOnCallEmail fetches the CSV and finds the email for the current time.
// It assumes the CSV has a header row and columns: Day (e.g., Monday), Start Time, End Time, Email.
// Times should be in Eastern Time (EST/EDT) and formatted like "3:00 AM" or "6:00 PM".
func (p *Provider) GetOnCallEmail() (string, error) {
	resp, err := http.Get(p.SheetURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch sheet: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch sheet, status: %d", resp.StatusCode)
	}

	reader := csv.NewReader(resp.Body)
	records, err := reader.ReadAll()
	if err != nil {
		return "", fmt.Errorf("failed to parse csv: %w", err)
	}

	if len(records) < 2 {
		return "", fmt.Errorf("csv is empty or missing data rows")
	}

	// Load Eastern Time (EST/EDT)
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return "", fmt.Errorf("failed to load timezone: %w", err)
	}

	now := p.TimeNow().In(loc)
	todayStr := now.Weekday().String() // e.g., "Friday"

	// Check if today is Saturday (work days are Sunday to Friday)
	if now.Weekday() == time.Saturday {
		return "", fmt.Errorf("today is Saturday, no one is on call")
	}

	// Check if outside of 3am to 6pm (18:00) window
	if now.Hour() < 3 || now.Hour() >= 18 {
		return "", fmt.Errorf("No one is on-call at the moment, we will reply to you as soon as possible!")
	}

	// Calculate the current time in seconds since midnight for easy comparison
	currentSeconds := now.Hour()*3600 + now.Minute()*60 + now.Second()

	// Columns expected: Day (0), Start Time (1), End Time (2), Email (3)
	for i, row := range records[1:] { // skip header
		if len(row) >= 4 {
			dayStr := strings.TrimSpace(row[0])
			if !strings.EqualFold(dayStr, todayStr) {
				log.Printf("Row %d: %q != %q", i, dayStr, todayStr)
				continue
			}

			startStr := strings.TrimSpace(row[1])
			endStr := strings.TrimSpace(row[2])
			email := strings.TrimSpace(row[3])

			// Parse times. We support "3:04 PM" layout.
			startTime, err := time.Parse("3:04 PM", strings.ToUpper(startStr))
			if err != nil {
				log.Printf("Row %d: Failed to parse start time %q: %v", i, startStr, err)
				continue // Skip rows with invalid time formats
			}
			endTime, err := time.Parse("3:04 PM", strings.ToUpper(endStr))
			if err != nil {
				log.Printf("Row %d: Failed to parse end time %q: %v", i, endStr, err)
				continue
			}

			startSec := startTime.Hour()*3600 + startTime.Minute()*60
			endSec := endTime.Hour()*3600 + endTime.Minute()*60

			log.Printf("Row %d checks: Email=%s, startSec=%d, endSec=%d, currentSec=%d", i, email, startSec, endSec, currentSeconds)

			if currentSeconds >= startSec && currentSeconds < endSec {
				log.Printf("Found match!")
				return email, nil
			}
		} else {
			log.Printf("Row %d has %d columns, expected at least 4", i, len(row))
		}
	}

	return "", fmt.Errorf("No one is on-call at the moment, we will reply to you as soon as possible!")
}
