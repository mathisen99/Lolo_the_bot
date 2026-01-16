package handler

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/user"
)

// MentionHandler handles bot mention detection and processing
type MentionHandler struct {
	apiClient                APIClientInterface
	userMgr                  *user.Manager
	db                       *database.DB
	botNick                  string
	testMode                 bool
	phoneNotificationsActive bool
	phoneNotificationsURL    string
}

func SendNotificationToPhone(message string, url string) {
	// Use curl to send a notification to the phone
	cmd := exec.Command("curl", "-d", message, url)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error sending notification: %v\n", err)
		return
	}
	fmt.Printf("Notification sent: %s\n", output)
}

// NewMentionHandler creates a new mention handler
func NewMentionHandler(apiClient APIClientInterface, userMgr *user.Manager, db *database.DB, botNick string, testMode bool, phoneNotificationsActive bool, phoneNotificationsURL string) *MentionHandler {
	return &MentionHandler{
		apiClient:                apiClient,
		userMgr:                  userMgr,
		db:                       db,
		botNick:                  botNick,
		testMode:                 testMode,
		phoneNotificationsActive: phoneNotificationsActive,
		phoneNotificationsURL:    phoneNotificationsURL,
	}
}

// UpdateBotNick updates the bot's current nickname
func (h *MentionHandler) UpdateBotNick(nick string) {
	h.botNick = nick
}

// ContainsMention checks if a message contains a mention of the bot
// Only matches if the nickname is a complete word (surrounded by whitespace or at boundaries)
func (h *MentionHandler) ContainsMention(message string) bool {
	if h.botNick == "" {
		return false
	}

	// Case-insensitive check
	lowerMessage := strings.ToLower(message)
	lowerNick := strings.ToLower(h.botNick)

	// Split message into words (by whitespace only)
	words := strings.Fields(lowerMessage)

	for _, word := range words {
		// Strip common trailing punctuation from the word
		word = strings.TrimRight(word, ".,!?;:'\"")

		if word == lowerNick {
			return true
		}
	}

	return false
}

// HandleMention processes a bot mention
// Returns the response message and any error
// If statusCallback is provided, it will be called with intermediate status updates
func (h *MentionHandler) HandleMention(ctx context.Context, message, nick, hostmask, channel string, statusCallback func(string)) (string, error) {
	// Check if channel is enabled
	enabled, err := h.db.GetChannelState(channel)
	if err != nil {
		return "", fmt.Errorf("failed to check channel state: %w", err)
	}
	if !enabled {
		// Channel is disabled, don't process mention
		return "", nil
	}

	// Check if user is ignored
	user, err := h.userMgr.GetUser(nick)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	// If user exists and is ignored, don't process mention
	if user != nil && user.Level == database.LevelIgnored {
		return "", nil
	}

	// Determine permission level string for API
	permissionLevel := "normal" // Default for unregistered users
	if user != nil {
		switch user.Level {
		case database.LevelOwner:
			permissionLevel = "owner"
		case database.LevelAdmin:
			permissionLevel = "admin"
		case database.LevelNormal:
			permissionLevel = "normal"
		case database.LevelIgnored:
			permissionLevel = "ignored"
		}
	}

	// In test mode, return a mock response
	if h.testMode {
		return fmt.Sprintf("%s: This is a test mode response to your mention!", nick), nil
	}

	// Send notification to phone if active
	if h.phoneNotificationsActive && h.phoneNotificationsURL != "" {
		fmt.Println("Sending notification to phone")
		message_built_for_phone := fmt.Sprintf("%s: %s", nick, message)
		SendNotificationToPhone(message_built_for_phone, h.phoneNotificationsURL)
	}

	// Retrieve last 20 messages from the channel for context
	conversationHistory, err := h.getConversationHistory(channel, 20)
	if err != nil {
		// Log error but continue without history
		fmt.Printf("Warning: Failed to retrieve conversation history: %v\n", err)
		conversationHistory = nil
	}

	// Send streaming mention to Python API
	startTime := time.Now()

	// Use SendMentionStream to get updates
	respChan, err := h.apiClient.SendMentionStream(ctx, message, nick, hostmask, channel, permissionLevel, conversationHistory)
	if err != nil {
		// Record error metric (Requirement 30.3)
		if errRecordErr := h.db.RecordError("mention_api_failed"); errRecordErr != nil {
			fmt.Printf("Warning: Failed to record error metric: %v\n", errRecordErr)
		}
		return "", fmt.Errorf("failed to send mention to API: %w", err)
	}

	// Process stream
	var finalMessage string
	var lastStatus string

	for resp := range respChan {
		fmt.Printf("[MentionHandler] Received chunk at %s: status=%s\n", time.Now().Format(time.RFC3339), resp.Status)
		lastStatus = resp.Status

		switch resp.Status {
		case "processing":
			if statusCallback != nil && resp.Message != "" {
				statusCallback(resp.Message)
			}
		case "success":
			finalMessage = resp.Message
		case "error":
			// Record error metric
			if errRecordErr := h.db.RecordError("mention_api_error"); errRecordErr != nil {
				fmt.Printf("Warning: Failed to record error metric: %v\n", errRecordErr)
			}
			return "", fmt.Errorf("API returned error [%s]: %s", resp.RequestID, resp.Message)
		case "null":
			// User requested silence
			return "", nil
		}
	}

	// Calculate total latency
	latencyMs := float64(time.Since(startTime).Milliseconds())

	// Record API latency metric
	if err := h.db.RecordAPILatency(latencyMs); err != nil {
		fmt.Printf("Warning: Failed to record API latency metric: %v\n", err)
	}

	if lastStatus == "" {
		// Stream closed without any response
		return "", fmt.Errorf("API stream closed without response")
	}

	return finalMessage, nil
}

// getConversationHistory retrieves recent messages from a channel for context
func (h *MentionHandler) getConversationHistory(channel string, limit int) ([]*database.Message, error) {
	filter := &database.MessageFilter{
		Channel: channel,
		Limit:   limit,
	}

	messages, err := h.db.QueryMessages(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}

	// Reverse the messages so they're in chronological order (oldest first)
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// ShouldProcessMention checks if a mention should be processed
// This performs all the filtering checks without actually processing the mention
func (h *MentionHandler) ShouldProcessMention(nick, channel string) (bool, error) {
	// Check if channel is enabled
	enabled, err := h.db.GetChannelState(channel)
	if err != nil {
		return false, fmt.Errorf("failed to check channel state: %w", err)
	}
	if !enabled {
		return false, nil
	}

	// Check if user is ignored
	user, err := h.userMgr.GetUser(nick)
	if err != nil {
		return false, fmt.Errorf("failed to get user: %w", err)
	}

	// If user exists and is ignored, don't process mention
	if user != nil && user.Level == database.LevelIgnored {
		return false, nil
	}

	return true, nil
}
