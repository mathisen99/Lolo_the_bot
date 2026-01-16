package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yourusername/lolo/internal/commands"
	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/errors"
	"github.com/yourusername/lolo/internal/ircformat"
	"github.com/yourusername/lolo/internal/output"
	"github.com/yourusername/lolo/internal/splitter"
	"github.com/yourusername/lolo/internal/user"
)

// MessageHandler handles all incoming IRC messages and routes them appropriately
type MessageHandler struct {
	dispatcher            *commands.Dispatcher
	mentionHandler        *MentionHandler
	apiClient             APIClientInterface
	userManager           *user.Manager
	db                    *database.DB
	logger                output.Logger
	errorHandler          *errors.ErrorHandler
	splitter              *splitter.Splitter
	splitTracker          *splitter.StateTracker // Tracks split message state for error recovery
	commandPrefix         string
	testMode              bool
	commandMetadata       map[string]*CommandMetadata // Cache of API command metadata
	imageDownloadChannels []string                    // Channels to auto-download images from
}

// MessageHandlerConfig contains configuration for the message handler
type MessageHandlerConfig struct {
	Dispatcher               *commands.Dispatcher
	APIClient                APIClientInterface
	UserManager              *user.Manager
	DB                       *database.DB
	Logger                   output.Logger
	ErrorHandler             *errors.ErrorHandler
	Splitter                 *splitter.Splitter
	BotNick                  string
	CommandPrefix            string
	TestMode                 bool
	ImageDownloadChannels    []string
	PhoneNotificationsActive bool
	PhoneNotificationsURL    string
}

// NewMessageHandler creates a new message handler
func NewMessageHandler(config *MessageHandlerConfig) *MessageHandler {
	mentionHandler := NewMentionHandler(
		config.APIClient,
		config.UserManager,
		config.DB,
		config.BotNick,
		config.TestMode,
		config.PhoneNotificationsActive,
		config.PhoneNotificationsURL,
	)

	return &MessageHandler{
		dispatcher:            config.Dispatcher,
		mentionHandler:        mentionHandler,
		apiClient:             config.APIClient,
		userManager:           config.UserManager,
		db:                    config.DB,
		logger:                config.Logger,
		errorHandler:          config.ErrorHandler,
		splitter:              config.Splitter,
		splitTracker:          splitter.NewStateTracker(),
		commandPrefix:         config.CommandPrefix,
		testMode:              config.TestMode,
		commandMetadata:       make(map[string]*CommandMetadata),
		imageDownloadChannels: config.ImageDownloadChannels,
	}
}

// CacheCommandMetadata caches command metadata from the Python API
// This should be called during bot startup after the API health check
func (h *MessageHandler) CacheCommandMetadata(ctx context.Context) error {
	h.logger.Info("Fetching command metadata from Python API...")

	commandsResp, err := h.apiClient.GetCommands(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch command metadata: %w", err)
	}

	// Cache the metadata
	for i := range commandsResp.Commands {
		cmd := &commandsResp.Commands[i]
		h.commandMetadata[cmd.Name] = cmd
		h.logger.Info("Cached metadata for command: %s (permission: %s, timeout: %ds)",
			cmd.Name, cmd.RequiredPermission, cmd.Timeout)
	}

	h.logger.Success("Cached metadata for %d API commands", len(commandsResp.Commands))
	return nil
}

// GetCommandMetadata returns cached metadata for a command
func (h *MessageHandler) GetCommandMetadata(command string) *CommandMetadata {
	return h.commandMetadata[command]
}

// HandleMessage processes an incoming IRC message and returns responses to send
// This is the main entry point for message processing
// statusCallback is an optional function to send immediate status updates (user loop feedback)
func (h *MessageHandler) HandleMessage(ctx context.Context, nick, hostmask, channel, message string, isPM bool, statusCallback func(string)) ([]string, error) {
	// Log the incoming message
	if isPM {
		h.logger.PrivateMessage(nick, message)
	} else {
		h.logger.ChannelMessage(channel, nick, message)
	}

	// Store the message in the database
	if err := h.logMessage(nick, hostmask, channel, message, false); err != nil {
		// Log error but don't stop processing
		h.errorHandler.LogError(errors.NewDatabaseError("log message", err), "message logging")
		// Continue processing even if logging fails
	}

	// Check if message contains an image URL and download it (only for configured channels)
	if imageURL := ExtractImageURL(message); imageURL != "" {
		if h.shouldDownloadFromChannel(channel) {
			if err := h.downloadImage(imageURL); err != nil {
				h.logger.Warning("Failed to download image: %v", err)
			}
		}
	}

	// Check if this is a command
	if h.dispatcher.IsCommand(message) {
		return h.handleCommand(ctx, nick, hostmask, channel, message, isPM)
	}

	// Check if this is a mention (only in channels, not PMs)
	if !isPM && h.mentionHandler.ContainsMention(message) {
		return h.handleMention(ctx, nick, hostmask, channel, message, statusCallback)
	}

	// Not a command or mention, no response needed
	return nil, nil
}

// handleCommand processes a command message
func (h *MessageHandler) handleCommand(ctx context.Context, nick, hostmask, channel, message string, isPM bool) ([]string, error) {
	// Parse the command
	command, args := h.dispatcher.ParseCommand(message)
	if command == "" {
		return nil, nil
	}

	// Check if this is a core command
	if h.isCoreCommand(command) {
		return h.handleCoreCommand(ctx, nick, hostmask, channel, message, isPM)
	}

	// Otherwise, route to API
	return h.handleAPICommand(ctx, command, args, nick, hostmask, channel, isPM)
}

// handleCoreCommand processes a core command (implemented in Go)
func (h *MessageHandler) handleCoreCommand(_ context.Context, nick, hostmask, channel, message string, isPM bool) ([]string, error) {
	// Check if channel is enabled (for channel messages)
	if !isPM {
		enabled, err := h.db.GetChannelState(channel)
		if err != nil {
			dbErr := errors.NewDatabaseError("check channel state", err)
			return []string{h.errorHandler.Handle(dbErr)}, nil
		}
		if !enabled {
			// Channel is disabled, don't process command
			return nil, nil
		}
	}

	// Check if user is ignored
	user, err := h.userManager.GetUser(nick)
	if err != nil {
		dbErr := errors.NewDatabaseError("get user", err)
		return []string{h.errorHandler.Handle(dbErr)}, nil
	}

	if user != nil && user.Level == database.LevelIgnored {
		// User is ignored, don't process command
		return nil, nil
	}

	// Use the dispatcher to execute the command
	response, isCommand, err := h.dispatcher.Dispatch(nick, hostmask, channel, message, isPM)
	if err != nil {
		// Record error metric (Requirement 30.3)
		if errRecordErr := h.db.RecordError("command_execution"); errRecordErr != nil {
			h.logger.Warning("Failed to record error metric: %v", errRecordErr)
		}
		// Handle the error and return user-friendly message
		return []string{h.errorHandler.HandleWithContext(err, "command execution")}, nil
	}

	if !isCommand {
		return nil, nil
	}

	if response == nil {
		return nil, nil
	}

	// Record command usage metric (Requirement 30.1)
	command, _ := h.dispatcher.ParseCommand(message)
	if command != "" {
		if err := h.db.RecordCommandUsage(command); err != nil {
			h.logger.Warning("Failed to record command metric: %v", err)
		}
	}

	// Format and return the response
	return h.formatResponse(response, nick, channel, isPM)
}

// handleAPICommand processes a command via the Python API
func (h *MessageHandler) handleAPICommand(ctx context.Context, command string, args []string, nick, hostmask, channel string, isPM bool) ([]string, error) {
	// Check if channel is enabled (for channel messages)
	if !isPM {
		enabled, err := h.db.GetChannelState(channel)
		if err != nil {
			dbErr := errors.NewDatabaseError("check channel state", err)
			return []string{h.errorHandler.Handle(dbErr)}, nil
		}
		if !enabled {
			// Channel is disabled, don't process command
			return nil, nil
		}
	}

	// Check if user is ignored
	user, err := h.userManager.GetUser(nick)
	if err != nil {
		dbErr := errors.NewDatabaseError("get user", err)
		return []string{h.errorHandler.Handle(dbErr)}, nil
	}

	if user != nil && user.Level == database.LevelIgnored {
		// User is ignored, don't process command
		return nil, nil
	}

	// In test mode, return a mock response
	if h.testMode {
		mockResponse := fmt.Sprintf("Test mode: Command '%s' with args %v", command, args)
		return h.splitMessage(mockResponse), nil
	}

	// Check if command supports streaming
	metadata := h.GetCommandMetadata(command)
	if metadata != nil && metadata.Streaming {
		// Use streaming endpoint
		return h.handleStreamingAPICommand(ctx, command, args, nick, hostmask, channel, isPM)
	}

	// Get command timeout from metadata if available
	var timeout time.Duration
	if metadata != nil && metadata.Timeout > 0 {
		timeout = time.Duration(metadata.Timeout) * time.Second
	}

	// Record command usage metric (Requirement 30.1)
	if err := h.db.RecordCommandUsage(command); err != nil {
		h.logger.Warning("Failed to record command metric: %v", err)
	}

	// Send command to Python API (non-streaming) and measure latency
	startTime := time.Now()
	apiResp, err := h.apiClient.SendCommand(ctx, command, args, nick, hostmask, channel, isPM, timeout)
	latencyMs := float64(time.Since(startTime).Milliseconds())

	// Record API latency metric (Requirement 30.2)
	if err := h.db.RecordAPILatency(latencyMs); err != nil {
		h.logger.Warning("Failed to record API latency metric: %v", err)
	}

	if err != nil {
		// Record error metric (Requirement 30.3)
		if errRecordErr := h.db.RecordError("api_request_failed"); errRecordErr != nil {
			h.logger.Warning("Failed to record error metric: %v", errRecordErr)
		}
		// Handle API error with proper error type
		apiErr := errors.NewAPIError(err)
		return []string{h.errorHandler.Handle(apiErr)}, nil
	}

	// Check response status first to avoid logging unknown commands
	if apiResp.Status != "success" {
		// Silently ignore "Unknown command" errors to reduce noise
		if strings.Contains(apiResp.Message, "Unknown command") {
			return nil, nil
		}

		// Log the request/response with request ID for actual errors
		h.logger.Info("API command [%s]: '%s' from %s - status: %s (latency: %.0fms)", apiResp.RequestID, command, nick, apiResp.Status, latencyMs)

		// Record error metric (Requirement 30.3)
		if errRecordErr := h.db.RecordError("api_command_error"); errRecordErr != nil {
			h.logger.Warning("Failed to record error metric: %v", errRecordErr)
		}
		h.logger.Warning("API error [%s]: %s", apiResp.RequestID, apiResp.Message)

		return []string{apiResp.Message}, nil
	}

	// Log successful command execution
	h.logger.Info("API command [%s]: '%s' from %s - status: %s (latency: %.0fms)", apiResp.RequestID, command, nick, apiResp.Status, latencyMs)

	// Check if response requires specific permission level
	if apiResp.RequiredLevel != "" {
		requiredLevel := parsePermissionLevel(apiResp.RequiredLevel)
		hasPermission, err := h.userManager.CheckPermission(nick, hostmask, requiredLevel)
		if err != nil {
			dbErr := errors.NewDatabaseError("check permission", err)
			return []string{h.errorHandler.Handle(dbErr)}, nil
		}

		if !hasPermission {
			permErr := errors.NewPermissionError(requiredLevel)
			return []string{h.errorHandler.Handle(permErr)}, nil
		}
	}

	// Download any images in the API response (flux create/edit)
	if imageURL := ExtractImageURL(apiResp.Message); imageURL != "" {
		if err := h.downloadImage(imageURL); err != nil {
			h.logger.Warning("Failed to download API-generated image: %v", err)
		}
	}

	// Return the API response
	return h.splitMessage(apiResp.Message), nil
}

// handleStreamingAPICommand processes a streaming command via the Python API
func (h *MessageHandler) handleStreamingAPICommand(ctx context.Context, command string, args []string, nick, hostmask, channel string, isPM bool) ([]string, error) {
	// Get command timeout from metadata if available
	metadata := h.GetCommandMetadata(command)
	var timeout time.Duration
	if metadata != nil && metadata.Timeout > 0 {
		timeout = time.Duration(metadata.Timeout) * time.Second
	}

	// Send streaming command to Python API
	responseChan, err := h.apiClient.SendCommandStream(ctx, command, args, nick, hostmask, channel, isPM, timeout)
	if err != nil {
		// Handle API error with proper error type
		apiErr := errors.NewAPIError(err)
		return []string{h.errorHandler.Handle(apiErr)}, nil
	}

	var responses []string
	var requestID string
	var lastError string

	// Collect all chunks from the streaming response
	for apiResp := range responseChan {
		if requestID == "" {
			requestID = apiResp.RequestID
		}

		// Log each chunk
		h.logger.Info("API streaming chunk [%s]: status: %s, streaming: %v", apiResp.RequestID, apiResp.Status, apiResp.Streaming)

		// Check response status
		if apiResp.Status != "success" {
			h.logger.Warning("API error [%s]: %s", apiResp.RequestID, apiResp.Message)
			lastError = apiResp.Message
			continue
		}

		// Check if response requires specific permission level
		if apiResp.RequiredLevel != "" {
			requiredLevel := parsePermissionLevel(apiResp.RequiredLevel)
			hasPermission, err := h.userManager.CheckPermission(nick, hostmask, requiredLevel)
			if err != nil {
				dbErr := errors.NewDatabaseError("check permission", err)
				return []string{h.errorHandler.Handle(dbErr)}, nil
			}

			if !hasPermission {
				permErr := errors.NewPermissionError(requiredLevel)
				return []string{h.errorHandler.Handle(permErr)}, nil
			}
		}

		// Add message chunk to responses
		if apiResp.Message != "" {
			responses = append(responses, apiResp.Message)
		}
	}

	// If there was an error and no responses, return the error
	if len(responses) == 0 && lastError != "" {
		return []string{lastError}, nil
	}

	// Download any images in streaming responses (flux create/edit)
	for _, resp := range responses {
		if imageURL := ExtractImageURL(resp); imageURL != "" {
			if err := h.downloadImage(imageURL); err != nil {
				h.logger.Warning("Failed to download streaming API-generated image: %v", err)
			}
		}
	}

	// Return all collected responses
	return responses, nil
}

// handleMention processes a bot mention
func (h *MessageHandler) handleMention(ctx context.Context, nick, hostmask, channel, message string, statusCallback func(string)) ([]string, error) {
	h.logger.Info("Processing mention from %s in %s", nick, channel)

	response, err := h.mentionHandler.HandleMention(ctx, message, nick, hostmask, channel, statusCallback)
	if err != nil {
		h.logger.Error("Mention handling failed: %v", err)
		return nil, nil // Don't send error messages for mentions
	}

	if response == "" {
		return nil, nil
	}

	// Download any images the bot generated (flux create/edit)
	if imageURL := ExtractImageURL(response); imageURL != "" {
		if err := h.downloadImage(imageURL); err != nil {
			h.logger.Warning("Failed to download bot-generated image: %v", err)
		}
	}

	// Convert custom formatting tags to IRC control codes
	// This allows AI to use tags like <BOLD>text</BOLD> which become IRC formatting
	formattedResponse := ircformat.Format(response)

	// Split the response
	messages := h.splitMessage(formattedResponse)

	// Log bot's mention responses to database (strip IRC codes for clean logs)
	for _, msg := range messages {
		cleanMsg := ircformat.StripIRCCodes(msg)
		if err := h.logMessage("bot", "", channel, cleanMsg, true); err != nil {
			h.errorHandler.LogError(errors.NewDatabaseError("log bot mention response", err), "bot mention logging")
		}
	}

	return messages, nil
}

// isCoreCommand checks if a command is a core command (implemented in Go)
func (h *MessageHandler) isCoreCommand(command string) bool {
	_, exists := h.dispatcher.GetRegistry().Get(command)
	return exists
}

// formatResponse formats a command response for sending
func (h *MessageHandler) formatResponse(response *commands.Response, _ string, channel string, isPM bool) ([]string, error) {
	if response == nil || response.Message == "" {
		return nil, nil
	}

	// Split the message if needed
	messages := h.splitMessage(response.Message)

	// Log bot's own messages
	target := channel
	if isPM || response.SendAsPM {
		target = ""
	}
	if response.Target != "" {
		target = response.Target
	}

	for _, msg := range messages {
		if err := h.logMessage("bot", "", target, msg, true); err != nil {
			h.errorHandler.LogError(errors.NewDatabaseError("log bot message", err), "bot message logging")
		}
	}

	return messages, nil
}

// splitMessage splits a long message into multiple parts
// Also splits on newlines since IRC doesn't support multi-line messages
func (h *MessageHandler) splitMessage(message string) []string {
	if h.splitter == nil {
		return []string{message}
	}

	// First split on newlines (IRC doesn't support multi-line messages)
	lines := strings.Split(message, "\n")

	var allParts []string
	for _, line := range lines {
		// Skip empty lines
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Then split each line if it's too long
		parts := h.splitter.Split(line)
		allParts = append(allParts, parts...)
	}

	if len(allParts) > 1 {
		h.logger.Info("Split message into %d parts", len(allParts))
	}

	return allParts
}

// logMessage stores a message in the database
func (h *MessageHandler) logMessage(nick, hostmask, channel, content string, isBot bool) error {
	// For registered users, try to get cached hostmask if not provided
	if hostmask == "" && !isBot {
		isRegistered, err := h.userManager.IsRegisteredUser(nick)
		if err != nil {
			return fmt.Errorf("failed to check if user is registered: %w", err)
		}

		if isRegistered {
			hostmask = h.userManager.GetCachedHostmask(nick)
		}
		// For unregistered users, hostmask remains empty (NULL in database)
	}

	msg := &database.Message{
		Timestamp: time.Now(),
		Channel:   channel,
		Nick:      nick,
		Hostmask:  hostmask,
		Content:   content,
		IsBot:     isBot,
	}

	return h.db.LogMessage(msg)
}

// UpdateBotNick updates the bot's current nickname
func (h *MessageHandler) UpdateBotNick(nick string) {
	h.mentionHandler.UpdateBotNick(nick)
}

// shouldDownloadFromChannel checks if images should be downloaded from a channel
func (h *MessageHandler) shouldDownloadFromChannel(channel string) bool {
	for _, ch := range h.imageDownloadChannels {
		if strings.EqualFold(ch, channel) {
			return true
		}
	}
	return false
}

// parsePermissionLevel converts a string permission level to database.PermissionLevel
func parsePermissionLevel(level string) database.PermissionLevel {
	switch strings.ToLower(level) {
	case "owner":
		return database.LevelOwner
	case "admin":
		return database.LevelAdmin
	case "normal":
		return database.LevelNormal
	case "ignored":
		return database.LevelIgnored
	default:
		return database.LevelNormal
	}
}

// SendSplitMessage sends a message that may need to be split, with error recovery
// Returns the split operation ID for tracking, and any error that occurred
func (h *MessageHandler) SendSplitMessage(ctx context.Context, target, message string, sendFunc func(target, message string) error) (string, error) {
	// Generate unique ID for this split operation
	splitID := uuid.New().String()

	// Check if message needs splitting
	if h.splitter == nil || !h.splitter.NeedsSplit(message) {
		// Message doesn't need splitting, send directly
		if err := sendFunc(target, message); err != nil {
			h.logger.Error("Failed to send message to %s: %v", target, err)
			h.errorHandler.LogError(
				fmt.Errorf("send message failed: %w", err),
				fmt.Sprintf("send to %s", target),
			)
			return splitID, err
		}
		return splitID, nil
	}

	// Split the message
	parts := h.splitter.Split(message)
	h.logger.Info("Split message into %d parts for %s (split ID: %s)", len(parts), target, splitID)

	// Track the split operation
	_ = h.splitTracker.StartSplit(splitID, target, message, parts)

	// Send each part
	for i, part := range parts {
		if err := sendFunc(target, part); err != nil {
			// Record the failure
			h.logger.Error("Failed to send part %d/%d to %s: %v", i+1, len(parts), target, err)

			// Log split failure with context (Requirement 24.7)
			h.errorHandler.LogError(
				fmt.Errorf("split message send failed at part %d/%d: %w", i+1, len(parts), err),
				fmt.Sprintf("split send to %s (ID: %s)", target, splitID),
			)

			// Mark the split as failed
			if i == 0 {
				// All parts failed
				failErr := splitter.NewAllPartsFailed(len(parts), err.Error())
				_ = h.splitTracker.MarkSplitFailed(splitID, failErr.Error())

				// Send error message to user (Requirement 24.7)
				errMsg := fmt.Sprintf("Error: Failed to send response (all %d parts failed). Please try again.", len(parts))
				if err := sendFunc(target, errMsg); err != nil {
					h.logger.Error("Failed to send error message: %v", err)
				}

				return splitID, failErr
			} else {
				// Partial failure
				failErr := splitter.NewPartialSendFailure(i, len(parts), err.Error())
				_ = h.splitTracker.MarkSplitFailed(splitID, failErr.Error())

				// Send error message to user (Requirement 24.7)
				errMsg := fmt.Sprintf("Error: Response partially sent (%d of %d parts). Last error: %v", i, len(parts), err)
				if err := sendFunc(target, errMsg); err != nil {
					h.logger.Error("Failed to send error message: %v", err)
				}

				return splitID, failErr
			}
		}

		// Mark part as sent
		if err := h.splitTracker.MarkPartSent(splitID, i); err != nil {
			h.logger.Warning("Failed to mark part %d as sent: %v", i+1, err)
		}
	}

	// Mark split as complete
	if err := h.splitTracker.CompleteSplit(splitID); err != nil {
		h.logger.Warning("Failed to mark split as complete: %v", err)
	}

	h.logger.Success("Successfully sent split message (%d parts) to %s (split ID: %s)", len(parts), target, splitID)

	return splitID, nil
}

// GetSplitState retrieves the current state of a split operation
func (h *MessageHandler) GetSplitState(splitID string) *splitter.SplitState {
	return h.splitTracker.GetState(splitID)
}

// GetIncompleteSplits returns all incomplete splits (for recovery on restart)
func (h *MessageHandler) GetIncompleteSplits() []*splitter.SplitState {
	return h.splitTracker.GetIncompleteSplits()
}

// CleanupOldSplits removes splits that have been in progress for too long
func (h *MessageHandler) CleanupOldSplits(maxAge time.Duration) int {
	cleaned := h.splitTracker.CleanupOldSplits(maxAge)
	if cleaned > 0 {
		h.logger.Warning("Cleaned up %d old split operations", cleaned)
	}
	return cleaned
}

// imageURLRegex matches common image file extensions in URLs
var imageURLRegex = regexp.MustCompile(`https?://\S+\.(?:png|jpg|jpeg|gif|webp|bmp|tiff|svg|ico|heic|heif|jfif|pjpeg|pjp|avif|apng|jxl|jpe|jif|jfi)`)

// downloadImage downloads an image from a URL to the img/ folder in the project root
// downloadImage downloads an image from a URL to the img/ folder
func (h *MessageHandler) downloadImage(imageURL string) error {
	// Ensure img directory exists (relative to where bot runs)
	imgDir := "img"
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		return fmt.Errorf("failed to create img directory: %w", err)
	}

	// Create request with browser-like headers to bypass bot protection
	req, err := http.NewRequest("GET", imageURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers to mimic a real browser request
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "identity") // Don't request compression for images
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Sec-Fetch-Dest", "image")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Referer", imageURL) // Some hosts check referer

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	// Extract filename from URL and add timestamp to avoid duplicates
	originalName := filepath.Base(imageURL)
	ext := filepath.Ext(originalName)
	baseName := strings.TrimSuffix(originalName, ext)
	timestamp := time.Now().UnixNano()
	filename := fmt.Sprintf("%s_%d%s", baseName, timestamp, ext)
	destPath := filepath.Join(imgDir, filename)

	// Create the destination file directly
	outFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	// Copy response body to file
	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write image: %w", err)
	}

	h.logger.Success("Downloaded image: %s", destPath)
	return nil
}

// ContainsImageURL checks if a message contains an image URL
func ContainsImageURL(message string) bool {
	return imageURLRegex.MatchString(message)
}

// ExtractImageURL extracts the first image URL from a message
func ExtractImageURL(message string) string {
	return imageURLRegex.FindString(message)
}
