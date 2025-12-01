package irc

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/yourusername/lolo/internal/output"
	"gopkg.in/irc.v4"
)

// CTCPHandler handles CTCP (Client-To-Client Protocol) requests
type CTCPHandler struct {
	client  *Client
	logger  output.Logger
	version string
}

// NewCTCPHandler creates a new CTCP handler
func NewCTCPHandler(client *Client, logger output.Logger, version string) *CTCPHandler {
	return &CTCPHandler{
		client:  client,
		logger:  logger,
		version: version,
	}
}

// HandleCTCP processes CTCP requests embedded in PRIVMSG
// CTCP messages are formatted as: \x01COMMAND [args]\x01
// Returns (handled, strippedMessage) where:
// - handled: true if CTCP was processed and should not continue as regular message
// - strippedMessage: the message with CTCP formatting removed (for ACTION messages)
func (h *CTCPHandler) HandleCTCP(msg *irc.Message) (handled bool, strippedMessage string) {
	if len(msg.Params) < 2 {
		return false, ""
	}

	message := msg.Trailing()

	// Check if this is a CTCP message (starts and ends with \x01)
	if !strings.HasPrefix(message, "\x01") || !strings.HasSuffix(message, "\x01") {
		return false, ""
	}

	// Extract CTCP command and arguments
	ctcpContent := strings.Trim(message, "\x01")
	parts := strings.SplitN(ctcpContent, " ", 2)
	command := strings.ToUpper(parts[0])

	var args string
	if len(parts) > 1 {
		args = parts[1]
	}

	sender := msg.Name

	// Handle different CTCP commands
	switch command {
	case "VERSION":
		h.handleVersion(sender)
		return true, ""

	case "PING":
		h.handlePing(sender, args)
		return true, ""

	case "TIME":
		h.handleTime(sender)
		return true, ""

	case "ACTION":
		// ACTION is a special CTCP that represents /me commands
		// These should be logged but don't need a reply
		// Return the stripped message so it can be logged to database
		h.logger.Info("* %s %s", sender, args)
		// Format as "* username action" for database storage
		return false, fmt.Sprintf("* %s %s", sender, args)

	default:
		// Log unhandled CTCP requests
		h.logger.Info("Unhandled CTCP request from %s: %s", sender, command)
		return true, ""
	}
}

// handleVersion responds to CTCP VERSION requests
func (h *CTCPHandler) handleVersion(sender string) {
	h.logger.Info("CTCP VERSION request from %s", sender)

	// Format: Bot Name, Version, Platform
	versionString := fmt.Sprintf("Lolo IRC Bot %s / %s %s",
		h.version,
		runtime.GOOS,
		runtime.GOARCH,
	)

	h.sendCTCPReply(sender, "VERSION", versionString)
}

// handlePing responds to CTCP PING requests
func (h *CTCPHandler) handlePing(sender, timestamp string) {
	h.logger.Info("CTCP PING request from %s", sender)

	// Echo back the same timestamp for latency measurement
	h.sendCTCPReply(sender, "PING", timestamp)
}

// handleTime responds to CTCP TIME requests
func (h *CTCPHandler) handleTime(sender string) {
	h.logger.Info("CTCP TIME request from %s", sender)

	// Return current date and time in RFC1123 format
	timeString := time.Now().Format(time.RFC1123)

	h.sendCTCPReply(sender, "TIME", timeString)
}

// sendCTCPReply sends a CTCP reply via NOTICE
// CTCP replies are sent as NOTICE messages with \x01 delimiters
func (h *CTCPHandler) sendCTCPReply(target, command, response string) {
	// Format: \x01COMMAND response\x01
	ctcpMessage := fmt.Sprintf("\x01%s %s\x01", command, response)

	err := h.client.Write(&irc.Message{
		Command: "NOTICE",
		Params:  []string{target, ctcpMessage},
	})

	if err != nil {
		h.logger.Error("Failed to send CTCP %s reply to %s: %v", command, target, err)
	}
}

// FormatCTCPMessage formats a CTCP message with proper delimiters
func FormatCTCPMessage(command, args string) string {
	if args == "" {
		return fmt.Sprintf("\x01%s\x01", command)
	}
	return fmt.Sprintf("\x01%s %s\x01", command, args)
}

// IsCTCPMessage checks if a message is a CTCP message
func IsCTCPMessage(message string) bool {
	return strings.HasPrefix(message, "\x01") && strings.HasSuffix(message, "\x01")
}

// ParseCTCPMessage extracts the command and arguments from a CTCP message
func ParseCTCPMessage(message string) (command, args string, ok bool) {
	if !IsCTCPMessage(message) {
		return "", "", false
	}

	ctcpContent := strings.Trim(message, "\x01")
	parts := strings.SplitN(ctcpContent, " ", 2)
	command = strings.ToUpper(parts[0])

	if len(parts) > 1 {
		args = parts[1]
	}

	return command, args, true
}
