// Package reminder handles checking and delivering reminders on IRC events.
package reminder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/yourusername/lolo/internal/output"
)

// IRCSender can send messages to IRC channels/users
type IRCSender interface {
	SendMessage(target, message string) error
}

// CheckJoinRequest is the request to the Python API
type CheckJoinRequest struct {
	Nick    string `json:"nick"`
	Channel string `json:"channel"`
}

// CheckJoinResponse is the response from the Python API
type CheckJoinResponse struct {
	Messages []string `json:"messages"`
}

// Checker checks for pending reminders via the Python API
type Checker struct {
	apiEndpoint string
	sender      IRCSender
	logger      output.Logger
	httpClient  *http.Client
}

// NewChecker creates a new reminder checker
func NewChecker(apiEndpoint string, sender IRCSender, logger output.Logger) *Checker {
	return &Checker{
		apiEndpoint: apiEndpoint,
		sender:      sender,
		logger:      logger,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// OnJoin checks for pending on-join reminders for a user.
// This should be called asynchronously (in a goroutine) to avoid blocking the IRC event loop.
func (c *Checker) OnJoin(nick, channel string) {
	req := CheckJoinRequest{
		Nick:    nick,
		Channel: channel,
	}

	body, err := json.Marshal(req)
	if err != nil {
		c.logger.Warning("Reminder: failed to marshal join check request: %v", err)
		return
	}

	resp, err := c.httpClient.Post(
		c.apiEndpoint+"/reminders/check_join",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		// Don't log connection errors — API might just be down
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var result CheckJoinResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		c.logger.Warning("Reminder: failed to decode join check response: %v", err)
		return
	}

	// Deliver each reminder message to IRC
	for _, msg := range result.Messages {
		if err := c.sender.SendMessage(channel, msg); err != nil {
			c.logger.Warning("Reminder: failed to deliver to %s: %v", channel, err)
		} else {
			c.logger.Info("Reminder: delivered on-join reminder to %s in %s", nick, channel)
		}
		// Small delay between messages to avoid flooding
		if len(result.Messages) > 1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// OnJoinAsync is a convenience wrapper that runs OnJoin in a goroutine
func (c *Checker) OnJoinAsync(nick, channel string) {
	go c.OnJoin(nick, channel)
}

// FormatReminderDelivery formats a reminder for IRC delivery
func FormatReminderDelivery(targetNick, creatorNick, message string) string {
	if targetNick == creatorNick {
		return fmt.Sprintf("%s: Reminder: %s", targetNick, message)
	}
	return fmt.Sprintf("%s: Reminder from %s: %s", targetNick, creatorNick, message)
}
