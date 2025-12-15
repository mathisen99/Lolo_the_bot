package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/user"
)

// MentionHandler handles bot mention detection and processing
type MentionHandler struct {
	apiClient APIClientInterface
	userMgr   *user.Manager
	db        *database.DB
	botNick   string
	testMode  bool
}

// NewMentionHandler creates a new mention handler
func NewMentionHandler(apiClient APIClientInterface, userMgr *user.Manager, db *database.DB, botNick string, testMode bool) *MentionHandler {
	return &MentionHandler{
		apiClient: apiClient,
		userMgr:   userMgr,
		db:        db,
		botNick:   botNick,
		testMode:  testMode,
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
func (h *MentionHandler) HandleMention(ctx context.Context, message, nick, hostmask, channel string) (string, error) {
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

	// Retrieve last 20 messages from the channel for context
	conversationHistory, err := h.getConversationHistory(channel, 20)
	if err != nil {
		// Log error but continue without history
		fmt.Printf("Warning: Failed to retrieve conversation history: %v\n", err)
		conversationHistory = nil
	}

	// Send mention to Python API with conversation history and measure latency
	startTime := time.Now()
	resp, err := h.apiClient.SendMention(ctx, message, nick, hostmask, channel, permissionLevel, conversationHistory)
	latencyMs := float64(time.Since(startTime).Milliseconds())

	// Record API latency metric (Requirement 30.2)
	if err := h.db.RecordAPILatency(latencyMs); err != nil {
		// Log but don't fail
		fmt.Printf("Warning: Failed to record API latency metric: %v\n", err)
	}

	if err != nil {
		// Record error metric (Requirement 30.3)
		if errRecordErr := h.db.RecordError("mention_api_failed"); errRecordErr != nil {
			fmt.Printf("Warning: Failed to record error metric: %v\n", errRecordErr)
		}
		return "", fmt.Errorf("failed to send mention to API [request ID unavailable]: %w", err)
	}

	// Check response status
	if resp.Status == "null" {
		// Null response - user requested silence, return empty to skip IRC message
		return "", nil
	}

	if resp.Status != "success" {
		// Record error metric (Requirement 30.3)
		if errRecordErr := h.db.RecordError("mention_api_error"); errRecordErr != nil {
			fmt.Printf("Warning: Failed to record error metric: %v\n", errRecordErr)
		}
		return "", fmt.Errorf("API returned error [%s]: %s", resp.RequestID, resp.Message)
	}

	return resp.Message, nil
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
