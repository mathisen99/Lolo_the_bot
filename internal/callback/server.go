// Package callback provides an HTTP server for Python API to call back into the Go bot.
// This enables tools like irc_command to execute IRC operations.
package callback

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/output"
	"gopkg.in/irc.v4"
)

// IRCExecutor interface for executing IRC commands
type IRCExecutor interface {
	Write(msg *irc.Message) error
	SendMessage(target, message string) error
}

// ResponseCollector collects responses from IRC services
type ResponseCollector struct {
	responses []string
	done      chan struct{}
	mu        sync.Mutex
	timeout   time.Duration
}

// Server handles callback requests from Python API
type Server struct {
	executor        IRCExecutor
	logger          output.Logger
	port            int
	server          *http.Server
	db              *database.DB
	pendingRequests map[string]*ResponseCollector
	mu              sync.RWMutex
}

// NewServer creates a new callback server
func NewServer(executor IRCExecutor, logger output.Logger, port int) *Server {
	return &Server{
		executor:        executor,
		logger:          logger,
		port:            port,
		pendingRequests: make(map[string]*ResponseCollector),
	}
}

// SetDatabase sets the database reference for channel status queries
func (s *Server) SetDatabase(db *database.DB) {
	s.db = db
}

// IRCExecuteRequest represents a request to execute an IRC command
type IRCExecuteRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Channel string   `json:"channel,omitempty"`
}

// IRCExecuteResponse represents the response from an IRC command
type IRCExecuteResponse struct {
	Status string `json:"status"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Start starts the HTTP callback server
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/irc/execute", s.handleIRCExecute)
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	s.logger.Info("Starting callback server on port %d", s.port)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Callback server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully stops the callback server
func (s *Server) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleIRCExecute handles IRC command execution requests
func (s *Server) handleIRCExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req IRCExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, "Invalid request body: "+err.Error())
		return
	}

	s.logger.Info("IRC callback: %s %v", req.Command, req.Args)

	// Execute the command and collect response
	output, err := s.executeCommand(req)
	if err != nil {
		s.sendError(w, err.Error())
		return
	}

	s.sendSuccess(w, output)
}

// executeCommand executes an IRC command and returns the output
func (s *Server) executeCommand(req IRCExecuteRequest) (string, error) {
	cmd := strings.ToLower(req.Command)

	switch cmd {
	// User info commands
	case "whois":
		return s.executeWHOIS(req.Args)
	case "whowas":
		return s.executeWHOWAS(req.Args)
	case "version":
		return s.executeCTCP(req.Args, "VERSION")
	case "time":
		return s.executeCTCP(req.Args, "TIME")

	// NickServ commands
	case "ns_info", "nickserv_info":
		return s.executeNickServInfo(req.Args)
	case "ns_ghost":
		return s.executeNickServCommand("GHOST", req.Args)
	case "ns_release":
		return s.executeNickServCommand("RELEASE", req.Args)
	case "ns_regain":
		return s.executeNickServCommand("REGAIN", req.Args)

	// ChanServ commands
	case "cs_info", "chanserv_info":
		return s.executeChanServInfo(req.Args)
	case "cs_op":
		return s.executeChanServCommand("OP", req.Args)
	case "cs_deop":
		return s.executeChanServCommand("DEOP", req.Args)
	case "cs_voice":
		return s.executeChanServCommand("VOICE", req.Args)
	case "cs_devoice":
		return s.executeChanServCommand("DEVOICE", req.Args)
	case "cs_kick":
		return s.executeChanServCommand("KICK", req.Args)
	case "cs_ban":
		return s.executeChanServCommand("BAN", req.Args)
	case "cs_unban":
		return s.executeChanServCommand("UNBAN", req.Args)
	case "cs_quiet":
		return s.executeChanServCommand("QUIET", req.Args)
	case "cs_unquiet":
		return s.executeChanServCommand("UNQUIET", req.Args)
	case "cs_topic":
		return s.executeChanServCommand("TOPIC", req.Args)
	case "cs_flags":
		return s.executeChanServCommand("FLAGS", req.Args)
	case "cs_access":
		return s.executeChanServCommand("ACCESS", req.Args)
	case "cs_akick":
		return s.executeChanServCommand("AKICK", req.Args)
	case "cs_invite":
		return s.executeChanServCommand("INVITE", req.Args)
	case "cs_clear":
		return s.executeChanServCommand("CLEAR", req.Args)

	// ALIS channel search
	case "alis_list", "alis_search":
		return s.executeALIS(req.Args)

	// Direct channel commands (bot must have op)
	case "kick":
		return s.executeKick(req.Args)
	case "ban":
		return s.executeMode(req.Args, "+b")
	case "unban":
		return s.executeMode(req.Args, "-b")
	case "quiet":
		return s.executeMode(req.Args, "+q")
	case "unquiet":
		return s.executeMode(req.Args, "-q")
	case "op":
		return s.executeMode(req.Args, "+o")
	case "deop":
		return s.executeMode(req.Args, "-o")
	case "voice":
		return s.executeMode(req.Args, "+v")
	case "devoice":
		return s.executeMode(req.Args, "-v")
	case "halfop":
		return s.executeMode(req.Args, "+h")
	case "dehalfop":
		return s.executeMode(req.Args, "-h")
	case "topic":
		return s.executeTopic(req.Args)
	case "mode":
		return s.executeRawMode(req.Args)
	case "invite":
		return s.executeInvite(req.Args)

	// Send message to a channel/user (used by reminder scheduler, etc.)
	case "send_message":
		return s.executeSendMessage(req.Args)

	// Bot status commands (local database queries)
	case "bot_status":
		return s.executeBotStatus(req.Args)
	case "channel_info":
		return s.executeChannelInfo(req.Args)
	case "channel_list":
		return s.executeChannelList()
	case "user_status":
		return s.executeUserStatus(req.Args)
	case "channel_ops":
		return s.executeChannelOps(req.Args)
	case "channel_voiced":
		return s.executeChannelVoiced(req.Args)
	case "channel_topic":
		return s.executeChannelTopic(req.Args)
	case "find_user":
		return s.executeFindUser(req.Args)
	case "channel_users":
		return s.executeChannelUsers(req.Args)
	case "channel_regular_users":
		return s.executeChannelRegularUsers(req.Args)
	case "search_users":
		return s.executeSearchUsers(req.Args)

	default:
		return "", fmt.Errorf("unknown command: %s", req.Command)
	}
}

// sendSuccess sends a success response
func (s *Server) sendSuccess(w http.ResponseWriter, output string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(IRCExecuteResponse{
		Status: "success",
		Output: output,
	})
}

// sendError sends an error response
func (s *Server) sendError(w http.ResponseWriter, errMsg string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(IRCExecuteResponse{
		Status: "error",
		Error:  errMsg,
	})
}

// executeBotStatus returns the bot's status in a channel
func (s *Server) executeBotStatus(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("bot_status requires a channel argument")
	}

	if s.db == nil {
		return "", fmt.Errorf("database not available")
	}

	channel := args[0]
	status, err := s.db.GetBotChannelStatus(channel)
	if err != nil {
		return "", fmt.Errorf("failed to get bot status: %w", err)
	}

	if status == nil {
		return fmt.Sprintf("Bot is not in channel %s", channel), nil
	}

	if !status.IsJoined {
		return fmt.Sprintf("Bot is not currently in channel %s", channel), nil
	}

	// Build status string
	var modes []string
	if status.IsOp {
		modes = append(modes, "op")
	}
	if status.IsHalfop {
		modes = append(modes, "halfop")
	}
	if status.IsVoice {
		modes = append(modes, "voice")
	}

	modeStr := "none"
	if len(modes) > 0 {
		modeStr = strings.Join(modes, ", ")
	}

	return fmt.Sprintf("Bot status in %s: joined=yes, modes=%s, has_op=%v",
		channel, modeStr, status.IsOp), nil
}

// executeChannelInfo returns information about a channel
func (s *Server) executeChannelInfo(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("channel_info requires a channel argument")
	}

	if s.db == nil {
		return "", fmt.Errorf("database not available")
	}

	channel := args[0]
	status, err := s.db.GetBotChannelStatus(channel)
	if err != nil {
		return "", fmt.Errorf("failed to get channel info: %w", err)
	}

	if status == nil || !status.IsJoined {
		return fmt.Sprintf("Bot is not in channel %s, cannot provide info", channel), nil
	}

	// Build info string
	info := fmt.Sprintf("Channel %s: users=%d, ops=%d, voiced=%d",
		channel, status.UserCount, status.OpCount, status.VoiceCount)

	if status.Topic != "" {
		// Truncate topic if too long
		topic := status.Topic
		if len(topic) > 100 {
			topic = topic[:97] + "..."
		}
		info += fmt.Sprintf(", topic=\"%s\"", topic)
	}

	info += fmt.Sprintf(", bot_has_op=%v", status.IsOp)

	return info, nil
}

// executeChannelList returns a list of all channels the bot is in
func (s *Server) executeChannelList() (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("database not available")
	}

	statuses, err := s.db.GetAllBotChannelStatuses()
	if err != nil {
		return "", fmt.Errorf("failed to get channel list: %w", err)
	}

	if len(statuses) == 0 {
		return "Bot is not in any channels", nil
	}

	// Build summary
	totalUsers := 0
	channelNames := make([]string, 0, len(statuses))
	for _, s := range statuses {
		channelNames = append(channelNames, s.Channel)
		totalUsers += s.UserCount
	}

	return fmt.Sprintf("Bot is in %d channels with %d total users: %s",
		len(statuses), totalUsers, strings.Join(channelNames, ", ")), nil
}

// executeUserStatus returns a user's status in a channel (op, voice, etc.)
func (s *Server) executeUserStatus(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("user_status requires channel and nick arguments")
	}

	if s.db == nil {
		return "", fmt.Errorf("database not available")
	}

	channel := args[0]
	nick := args[1]

	user, err := s.db.GetChannelUser(channel, nick)
	if err != nil {
		return "", fmt.Errorf("failed to get user status: %w", err)
	}

	if user == nil {
		return fmt.Sprintf("User %s is not in channel %s (or not tracked)", nick, channel), nil
	}

	// Build status string
	var modes []string
	if user.IsOp {
		modes = append(modes, "op (@)")
	}
	if user.IsHalfop {
		modes = append(modes, "halfop (%%)")
	}
	if user.IsVoice {
		modes = append(modes, "voice (+)")
	}

	modeStr := "no special modes"
	if len(modes) > 0 {
		modeStr = strings.Join(modes, ", ")
	}

	return fmt.Sprintf("%s in %s: %s", nick, channel, modeStr), nil
}

// executeChannelOps returns list of ops in a channel
func (s *Server) executeChannelOps(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("channel_ops requires a channel argument")
	}

	if s.db == nil {
		return "", fmt.Errorf("database not available")
	}

	channel := args[0]
	users, err := s.db.GetChannelUsersByMode(channel, "op")
	if err != nil {
		return "", fmt.Errorf("failed to get channel ops: %w", err)
	}

	if len(users) == 0 {
		return fmt.Sprintf("No ops in %s", channel), nil
	}

	return fmt.Sprintf("Ops in %s (%d): %s", channel, len(users), strings.Join(users, ", ")), nil
}

// executeChannelVoiced returns list of voiced users in a channel
func (s *Server) executeChannelVoiced(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("channel_voiced requires a channel argument")
	}

	if s.db == nil {
		return "", fmt.Errorf("database not available")
	}

	channel := args[0]
	users, err := s.db.GetChannelUsersByMode(channel, "voice")
	if err != nil {
		return "", fmt.Errorf("failed to get voiced users: %w", err)
	}

	if len(users) == 0 {
		return fmt.Sprintf("No voiced users in %s", channel), nil
	}

	return fmt.Sprintf("Voiced in %s (%d): %s", channel, len(users), strings.Join(users, ", ")), nil
}

// executeChannelTopic returns just the topic of a channel
func (s *Server) executeChannelTopic(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("channel_topic requires a channel argument")
	}

	if s.db == nil {
		return "", fmt.Errorf("database not available")
	}

	channel := args[0]
	status, err := s.db.GetBotChannelStatus(channel)
	if err != nil {
		return "", fmt.Errorf("failed to get channel topic: %w", err)
	}

	if status == nil || !status.IsJoined {
		return fmt.Sprintf("Bot is not in channel %s", channel), nil
	}

	if status.Topic == "" {
		return fmt.Sprintf("No topic set in %s", channel), nil
	}

	return fmt.Sprintf("Topic for %s: %s", channel, status.Topic), nil
}

// executeFindUser searches for a user across all channels
func (s *Server) executeFindUser(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("find_user requires a nick argument")
	}

	if s.db == nil {
		return "", fmt.Errorf("database not available")
	}

	nick := args[0]
	channels, err := s.db.FindUserChannels(nick)
	if err != nil {
		return "", fmt.Errorf("failed to find user: %w", err)
	}

	if len(channels) == 0 {
		return fmt.Sprintf("User %s not found in any channels I'm in", nick), nil
	}

	return fmt.Sprintf("%s is in %d channel(s): %s", nick, len(channels), strings.Join(channels, ", ")), nil
}

// executeChannelUsers returns all users in a channel
func (s *Server) executeChannelUsers(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("channel_users requires a channel argument")
	}

	if s.db == nil {
		return "", fmt.Errorf("database not available")
	}

	channel := args[0]
	nicks, err := s.db.GetChannelUserNicks(channel)
	if err != nil {
		return "", fmt.Errorf("failed to get channel users: %w", err)
	}

	if len(nicks) == 0 {
		return fmt.Sprintf("No users tracked in %s (bot may not be in channel)", channel), nil
	}

	return fmt.Sprintf("Users in %s (%d): %s", channel, len(nicks), strings.Join(nicks, ", ")), nil
}

// executeChannelRegularUsers returns users without op/halfop/voice in a channel
func (s *Server) executeChannelRegularUsers(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("channel_regular_users requires a channel argument")
	}

	if s.db == nil {
		return "", fmt.Errorf("database not available")
	}

	channel := args[0]
	nicks, err := s.db.GetChannelRegularUsers(channel)
	if err != nil {
		return "", fmt.Errorf("failed to get regular users: %w", err)
	}

	if len(nicks) == 0 {
		return fmt.Sprintf("No regular users (without +o/+h/+v) in %s", channel), nil
	}

	return fmt.Sprintf("Regular users in %s (no +o/+h/+v) (%d): %s", channel, len(nicks), strings.Join(nicks, ", ")), nil
}

// executeSearchUsers searches for users by nick pattern
func (s *Server) executeSearchUsers(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("search_users requires a pattern argument (use %% as wildcard)")
	}

	if s.db == nil {
		return "", fmt.Errorf("database not available")
	}

	pattern := args[0]
	var channel string
	if len(args) >= 2 {
		channel = args[1]
	}

	// Convert * to % for SQL LIKE if user uses glob-style
	pattern = strings.ReplaceAll(pattern, "*", "%")

	// Ensure pattern has wildcards
	if !strings.Contains(pattern, "%") {
		pattern = "%" + pattern + "%"
	}

	if channel != "" {
		// Search in specific channel
		users, err := s.db.SearchChannelUsers(channel, pattern)
		if err != nil {
			return "", fmt.Errorf("failed to search users: %w", err)
		}

		if len(users) == 0 {
			return fmt.Sprintf("No users matching '%s' in %s", pattern, channel), nil
		}

		// Build result with modes
		var results []string
		for _, u := range users {
			prefix := ""
			if u.IsOp {
				prefix = "@"
			} else if u.IsHalfop {
				prefix = "%"
			} else if u.IsVoice {
				prefix = "+"
			}
			results = append(results, prefix+u.Nick)
		}

		return fmt.Sprintf("Users matching '%s' in %s (%d): %s", pattern, channel, len(users), strings.Join(results, ", ")), nil
	}

	// Search globally
	users, err := s.db.SearchUsersGlobal(pattern)
	if err != nil {
		return "", fmt.Errorf("failed to search users: %w", err)
	}

	if len(users) == 0 {
		return fmt.Sprintf("No users matching '%s' found", pattern), nil
	}

	// Group by nick with channels
	nickChannels := make(map[string][]string)
	for _, u := range users {
		nickChannels[u.Nick] = append(nickChannels[u.Nick], u.Channel)
	}

	var results []string
	for nick, channels := range nickChannels {
		results = append(results, fmt.Sprintf("%s (%s)", nick, strings.Join(channels, ", ")))
	}

	return fmt.Sprintf("Users matching '%s' (%d): %s", pattern, len(nickChannels), strings.Join(results, "; ")), nil
}

// executeSendMessage sends a PRIVMSG to a channel or user
// Args: [target, message]
func (s *Server) executeSendMessage(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("send_message requires target and message arguments")
	}

	target := args[0]
	message := strings.Join(args[1:], " ")

	if err := s.executor.SendMessage(target, message); err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	return fmt.Sprintf("Message sent to %s", target), nil
}
