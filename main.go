package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/omark/oncall-pager/internal/schedule"
	"github.com/omark/oncall-pager/internal/slackclient"
)

func main() {
	// Load .env file if it exists (ignore error if not found)
	_ = godotenv.Load()

	appToken := os.Getenv("SLACK_APP_TOKEN")
	botToken := os.Getenv("SLACK_BOT_TOKEN")
	scheduleFile := os.Getenv("SCHEDULE_FILE")
	intakeChannel := os.Getenv("INTAKE_CHANNEL_NAME")

	if scheduleFile == "" {
		scheduleFile = "schedule.json"
	}

	if appToken == "" || botToken == "" {
		log.Fatal("SLACK_APP_TOKEN and SLACK_BOT_TOKEN must be set")
	}

	if intakeChannel == "" {
		intakeChannel = "intake" // Default to intake
	}

	schedProvider := schedule.NewProvider(scheduleFile)

	client, err := slackclient.NewClient(appToken, botToken, intakeChannel, schedProvider)
	if err != nil {
		log.Fatalf("Failed to initialize Slack client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	log.Println("Starting On-Call Pager Bot...")
	if err := client.Start(ctx); err != nil {
		log.Fatalf("Error running Slack client: %v", err)
	}
}
