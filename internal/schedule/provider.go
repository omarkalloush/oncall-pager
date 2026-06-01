package schedule

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

type Provider struct {
	FilePath string
	TimeNow  func() time.Time
}

func NewProvider(filePath string) *Provider {
	return &Provider{
		FilePath: filePath,
		TimeNow:  time.Now,
	}
}

type ScheduleEntry struct {
	Day       string `json:"day"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Email     string `json:"email"`
}

// GetOnCallEmail reads the JSON file and finds the email for the current time.
// It assumes the JSON has an array of objects with keys: day (e.g., Monday), start_time, end_time, email.
// Times should be in Eastern Time (EST/EDT) and formatted like "3:00 AM" or "6:00 PM".
func (p *Provider) GetOnCallEmail() (string, error) {
	data, err := os.ReadFile(p.FilePath)
	if err != nil {
		return "", fmt.Errorf("failed to read schedule file: %w", err)
	}

	var records []ScheduleEntry
	if err := json.Unmarshal(data, &records); err != nil {
		return "", fmt.Errorf("failed to parse json: %w", err)
	}

	if len(records) == 0 {
		return "", fmt.Errorf("json is empty or missing data rows")
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

	for i, record := range records {
		dayStr := strings.TrimSpace(record.Day)
		if !strings.EqualFold(dayStr, todayStr) {
			log.Printf("Row %d: %q != %q", i, dayStr, todayStr)
			continue
		}

		startStr := strings.TrimSpace(record.StartTime)
		endStr := strings.TrimSpace(record.EndTime)
		email := strings.TrimSpace(record.Email)

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
	}

	return "", fmt.Errorf("No one is on-call at the moment, we will reply to you as soon as possible!")
}
