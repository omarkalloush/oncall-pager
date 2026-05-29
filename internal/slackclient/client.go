package slackclient

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type ScheduleProvider interface {
	GetOnCallEmail() (string, error)
}

type Client struct {
	api             *slack.Client
	socketMode      *socketmode.Client
	schedule        ScheduleProvider
	intakeChannel   string
	intakeChannelID string
	processedMsgs   sync.Map
}

func NewClient(appToken, botToken, intakeChannel string, schedule ScheduleProvider) (*Client, error) {
	api := slack.New(
		botToken,
		slack.OptionDebug(false),
		slack.OptionAppLevelToken(appToken),
	)

	socketClient := socketmode.New(
		api,
		socketmode.OptionDebug(false),
	)

	return &Client{
		api:           api,
		socketMode:    socketClient,
		schedule:      schedule,
		intakeChannel: intakeChannel,
	}, nil
}

func (c *Client) Start(ctx context.Context) error {
	// First, find the intake channel ID
	err := c.resolveChannelID()
	if err != nil {
		return fmt.Errorf("failed to resolve channel ID: %w", err)
	}

	log.Printf("Listening to intake channel: %s (ID: %s)", c.intakeChannel, c.intakeChannelID)

	go c.handleEvents(ctx)

	return c.socketMode.Run()
}

func (c *Client) resolveChannelID() error {
	var cursor string
	for {
		channels, nextCursor, err := c.api.GetConversations(&slack.GetConversationsParameters{
			Cursor:          cursor,
			ExcludeArchived: true,
			Types:           []string{"public_channel", "private_channel"},
		})
		if err != nil {
			return err
		}

		for _, channel := range channels {
			if channel.Name == c.intakeChannel || channel.Name == strings.TrimPrefix(c.intakeChannel, "#") {
				c.intakeChannelID = channel.ID
				return nil
			}
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return fmt.Errorf("channel %s not found", c.intakeChannel)
}

func (c *Client) handleEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-c.socketMode.Events:
			log.Printf("Received SocketMode event type: %s", evt.Type)

			switch evt.Type {
			case socketmode.EventTypeEventsAPI:
				eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					log.Printf("Failed to cast to EventsAPIEvent")
					continue
				}

				// Acknowledge the event
				c.socketMode.Ack(*evt.Request)

				log.Printf("EventsAPI Inner Type: %s", eventsAPIEvent.InnerEvent.Type)

				switch eventsAPIEvent.Type {
				case slackevents.CallbackEvent:
					innerEvent := eventsAPIEvent.InnerEvent
					switch ev := innerEvent.Data.(type) {
					case *slackevents.MessageEvent:
						// Handle message event concurrently
						go c.handleMessageEvent(ev)
					default:
						log.Printf("Received other inner event: %T", innerEvent.Data)
					}
				}
			}
		}
	}
}

func (c *Client) handleMessageEvent(ev *slackevents.MessageEvent) {
	log.Printf("MessageEvent details: Channel=%s, User=%s, BotID=%s, SubType=%s", ev.Channel, ev.User, ev.BotID, ev.SubType)

	// Deduplicate retries from Slack
	if ev.ClientMsgID != "" {
		if _, loaded := c.processedMsgs.LoadOrStore(ev.ClientMsgID, true); loaded {
			log.Printf("Ignoring duplicate message from Slack retries: %s", ev.ClientMsgID)
			return
		}
	}

	// Ignore messages not in the intake channel
	if ev.Channel != c.intakeChannelID {
		log.Printf("Ignoring message: wrong channel (got %s, expected %s)", ev.Channel, c.intakeChannelID)
		return
	}

	// Ignore messages from bots (including ourselves)
	if ev.BotID != "" {
		log.Printf("Ignoring message: it is a bot message")
		return
	}

	// Ignore special message subtypes (like message_changed, channel_join, etc.)
	if ev.SubType != "" {
		log.Printf("Ignoring message: subtype is %s", ev.SubType)
		return
	}

	// Ignore replies inside an existing thread so we only page for new top-level requests
	if ev.ThreadTimeStamp != "" && ev.ThreadTimeStamp != ev.TimeStamp {
		log.Printf("Ignoring message: it is a thread reply")
		return
	}

	log.Printf("Received message in intake channel from user %s", ev.User)

	email, err := c.schedule.GetOnCallEmail()
	if err != nil {
		log.Printf("Error getting on-call email: %v", err)
		
		errMsg := err.Error()
		if strings.Contains(errMsg, "No one is on-call") || strings.Contains(errMsg, "today is Saturday") {
			c.sendMessageToIntake(errMsg, ev.TimeStamp)
		} else {
			c.sendMessageToIntake(fmt.Sprintf("A new message was posted, but I couldn't find the on-call person: %v", err), ev.TimeStamp)
		}
		return
	}

	user, err := c.api.GetUserByEmail(email)
	if err != nil {
		log.Printf("Error finding Slack user for email %s: %v", email, err)
		c.sendMessageToIntake(fmt.Sprintf("A new message was posted! The on-call person is %s, but I couldn't find their Slack account.", email), ev.TimeStamp)
		return
	}

	// Notify the channel
	msg := fmt.Sprintf("<@%s>, a new message was posted in the intake channel by <@%s>!", user.ID, ev.User)
	c.sendMessageToIntake(msg, ev.TimeStamp)
}

func (c *Client) sendMessageToIntake(text string, threadTS string) {
	options := []slack.MsgOption{
		slack.MsgOptionText(text, false),
	}
	if threadTS != "" {
		options = append(options, slack.MsgOptionTS(threadTS))
	}

	_, _, err := c.api.PostMessage(
		c.intakeChannelID,
		options...,
	)
	if err != nil {
		log.Printf("Failed to post message to channel: %v", err)
	}
}
