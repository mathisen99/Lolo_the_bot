package irc

import (
	"fmt"
	"strings"
	"time"

	"github.com/yourusername/lolo/internal/config"
	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/output"
	"github.com/yourusername/lolo/internal/user"
	"gopkg.in/irc.v4"
)

// PrivMsgHandler is a callback function for handling PRIVMSG events
type PrivMsgHandler func(nick, hostmask, channel, message string, isPM bool)

// NoticeHandler is a callback function for handling NOTICE events (for service responses)
type NoticeHandler func(source, message string)

// NumericHandler is a callback function for handling numeric IRC responses
type NumericHandler func(numeric int, params []string)

// CTCPResponseHandler is a callback function for handling CTCP responses
type CTCPResponseHandler func(source, ctcpType, response string)

// ChannelUserTracker interface for tracking channel users
type ChannelUserTracker interface {
	OnNamesReply(channel string, names []string)
	OnJoin(channel, nick string, isSelf bool)
	OnPart(channel, nick string, isSelf bool)
	OnQuit(nick string)
	OnKick(channel, nick string, isSelf bool)
	OnMode(channel, nick, mode string, adding bool, isSelf bool)
	OnNickChange(oldNick, newNick string, isSelf bool)
	OnTopic(channel, topic string)
}

// ConnectionManager manages the IRC connection lifecycle
type ConnectionManager struct {
	client *Client
	auth   *Authenticator
	config *config.Config
	logger output.Logger
	db     *database.DB

	registered         bool
	currentNick        string
	altNickIndex       int
	underscoreCount    int
	primaryNickReclaim bool

	lastPingTime        time.Time
	lastPongTime        time.Time
	reconnectManager    *ReconnectionManager
	kickManager         *KickManager
	channelManager      *ChannelManager
	ctcpHandler         *CTCPHandler
	netsplitDetector    *NetsplitDetector
	privMsgHandler      PrivMsgHandler      // Callback for handling PRIVMSG events
	noticeHandler       NoticeHandler       // Callback for handling NOTICE events
	numericHandler      NumericHandler      // Callback for handling numeric responses
	ctcpResponseHandler CTCPResponseHandler // Callback for handling CTCP responses
	channelUserTracker  ChannelUserTracker  // Tracker for channel user state
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(cfg *config.Config, logger output.Logger, db *database.DB, userManager *user.Manager) *ConnectionManager {
	client := NewClient(cfg, logger)
	auth := NewAuthenticator(cfg, logger, client)

	cm := &ConnectionManager{
		client:             client,
		auth:               auth,
		config:             cfg,
		logger:             logger,
		db:                 db,
		currentNick:        cfg.Server.Nickname,
		altNickIndex:       -1,
		underscoreCount:    0,
		primaryNickReclaim: false,
		lastPongTime:       time.Now(),
	}

	// Set up message handler
	client.SetHandler(irc.HandlerFunc(cm.handleMessage))

	// Create reconnection manager
	cm.reconnectManager = NewReconnectionManager(cm, cfg, logger)

	// Create kick manager
	cm.kickManager = NewKickManager(logger)

	// Create channel manager
	cm.channelManager = NewChannelManager(db, logger, client, cfg.Bot.Channels)

	// Create CTCP handler (version 1.0.0 for now, can be made configurable later)
	cm.ctcpHandler = NewCTCPHandler(client, logger, "1.0.0")

	// Create netsplit detector
	cm.netsplitDetector = NewNetsplitDetector(logger, userManager)

	return cm
}

// Connect establishes connection and performs authentication
// Note: The event loop (Run) must be started BEFORE calling this method
func (cm *ConnectionManager) Connect() error {
	// Reset registration state for reconnection
	cm.registered = false

	// Load channel states from database
	err := cm.channelManager.LoadChannelStates()
	if err != nil {
		cm.logger.Warning("Failed to load channel states: %v", err)
		// Continue anyway with default states
	}

	// Connect to server
	err = cm.client.Connect()
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	// Wait for connection to be established
	time.Sleep(1 * time.Second)

	// Perform authentication
	err = cm.auth.Authenticate()
	if err != nil {
		cm.logger.Error("Authentication failed: %v", err)
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Wait for registration to complete (increased timeout for slower servers)
	// The event loop must be running to receive registration messages
	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("registration timed out after 60 seconds")
		case <-ticker.C:
			if cm.registered {
				cm.logger.Success("Registration complete")
				// Reset ping/pong tracking
				cm.lastPongTime = time.Now()
				return nil
			}
		}
	}
}

// StartReconnectionManager starts the automatic reconnection manager
func (cm *ConnectionManager) StartReconnectionManager() {
	cm.reconnectManager.Start()
	cm.logger.Info("Reconnection manager started")
}

// StopReconnectionManager stops the automatic reconnection manager
func (cm *ConnectionManager) StopReconnectionManager() {
	cm.reconnectManager.Stop()
	cm.logger.Info("Reconnection manager stopped")
}

// Disconnect closes the connection
func (cm *ConnectionManager) Disconnect() error {
	return cm.client.Disconnect()
}

// JoinChannels joins all configured auto-join channels (respecting enabled/disabled state)
func (cm *ConnectionManager) JoinChannels() error {
	return cm.channelManager.JoinAutoJoinChannels()
}

// RejoinChannels rejoins all previously joined channels (respecting enabled/disabled state)
func (cm *ConnectionManager) RejoinChannels() error {
	return cm.channelManager.RejoinChannels()
}

// GetClient returns the underlying IRC client
func (cm *ConnectionManager) GetClient() *Client {
	return cm.client
}

// IsConnected returns whether the client is connected
func (cm *ConnectionManager) IsConnected() bool {
	return cm.client.IsConnected()
}

// handleMessage processes incoming IRC messages
func (cm *ConnectionManager) handleMessage(client *irc.Client, msg *irc.Message) {
	// Debug: Log all messages during registration
	if !cm.registered {
		cm.logger.Info("IRC message during registration: %s %v", msg.Command, msg.Params)
	}

	// Handle SASL messages
	cm.auth.HandleSASLMessage(msg)

	switch msg.Command {
	case "001": // RPL_WELCOME
		cm.logger.Success("Welcome message received: %s", msg.Trailing())
		cm.registered = true

	case "002", "003", "004": // Server info messages
		cm.logger.Info("Server: %s", msg.Trailing())

	case "005": // RPL_ISUPPORT
		// Server capabilities
		cm.logger.Info("Server capabilities: %v", msg.Params)

	case "376", "422": // RPL_ENDOFMOTD, ERR_NOMOTD
		// End of MOTD - connection is fully established
		cm.logger.Success("Connection fully established")

		// Mark as registered if we haven't received 001 yet
		if !cm.registered {
			cm.logger.Info("Marking registration complete (via MOTD end)")
			cm.registered = true
		}

		// Join configured channels
		go func() {
			time.Sleep(1 * time.Second)
			_ = cm.JoinChannels()
		}()

	case "433": // ERR_NICKNAMEINUSE
		cm.handleNicknameInUse()

	case "474": // ERR_BANNEDFROMCHAN
		cm.handleBanError(msg)

	case "NICK":
		cm.handleNickChange(msg)

	case "QUIT":
		cm.handleQuit(msg)

	case "JOIN":
		cm.handleJoin(msg)

	case "PART":
		cm.handlePart(msg)

	case "KICK":
		cm.handleKick(msg)

	case "MODE":
		cm.handleMode(msg)

	case "TOPIC":
		cm.handleTopicChange(msg)

	case "PING":
		cm.handlePing(msg)

	case "PONG":
		cm.handlePong(msg)

	case "PRIVMSG":
		cm.handlePrivMsg(msg)

	case "NOTICE":
		cm.handleNotice(msg)

	case "ERROR":
		cm.logger.Error("IRC Error: %s", msg.Trailing())

	default:
		// Handle numeric responses (WHOIS, WHOWAS, etc.)
		cm.handleNumericIfApplicable(msg)
	}
}

// handleNicknameInUse handles nickname collision
func (cm *ConnectionManager) handleNicknameInUse() {
	cm.logger.Warning("Nickname %s is in use", cm.currentNick)

	var newNick string

	// If we haven't tried alternatives yet, start with the first one
	if cm.altNickIndex == -1 && len(cm.config.Server.AltNicknames) > 0 {
		cm.altNickIndex = 0
		newNick = cm.config.Server.AltNicknames[cm.altNickIndex]
		cm.logger.Info("Trying alternative nickname: %s", newNick)
	} else if cm.altNickIndex >= 0 && cm.altNickIndex < len(cm.config.Server.AltNicknames)-1 {
		// Try next alternative
		cm.altNickIndex++
		newNick = cm.config.Server.AltNicknames[cm.altNickIndex]
		cm.logger.Info("Trying alternative nickname: %s", newNick)
	} else {
		// All alternatives exhausted, append underscore
		cm.underscoreCount++
		baseNick := cm.config.Server.Nickname
		underscores := ""
		for i := 0; i < cm.underscoreCount; i++ {
			underscores += "_"
		}
		newNick = baseNick + underscores
		cm.logger.Warning("All alternatives exhausted, trying: %s", newNick)
	}

	cm.currentNick = newNick
	err := cm.client.Write(&irc.Message{
		Command: "NICK",
		Params:  []string{newNick},
	})
	if err != nil {
		cm.logger.Error("Failed to change nickname: %v", err)
	}
}

// handleNickChange handles NICK messages
func (cm *ConnectionManager) handleNickChange(msg *irc.Message) {
	if len(msg.Params) > 0 {
		oldNick := msg.Name
		newNick := msg.Params[0]
		hostmask := msg.User + "@" + msg.Host

		// Log the nick change event
		if cm.db != nil {
			content := fmt.Sprintf("%s is now known as %s", oldNick, newNick)
			if err := cm.db.LogEvent(database.EventTypeNickChange, "", oldNick, hostmask, content); err != nil {
				cm.logger.Warning("Failed to log nick change event: %v", err)
			}
		}

		// Check if it's our own nick change
		if oldNick == cm.currentNick {
			cm.currentNick = newNick
			cm.logger.Success("Nickname changed to: %s", newNick)

			// If we successfully got the primary nickname, reset state
			if newNick == cm.config.Server.Nickname {
				cm.altNickIndex = -1
				cm.underscoreCount = 0
				cm.primaryNickReclaim = false
				cm.logger.Success("Primary nickname reclaimed!")
			}

			// Notify tracker (bot nick change)
			if cm.channelUserTracker != nil {
				cm.channelUserTracker.OnNickChange(oldNick, newNick, true)
			}
		} else if oldNick == cm.config.Server.Nickname && newNick != cm.currentNick {
			// Someone else changed from our primary nickname - try to reclaim it
			cm.attemptPrimaryNickReclaim()

			// Notify tracker (other user nick change)
			if cm.channelUserTracker != nil {
				cm.channelUserTracker.OnNickChange(oldNick, newNick, false)
			}
		} else {
			// Notify tracker (other user nick change)
			if cm.channelUserTracker != nil {
				cm.channelUserTracker.OnNickChange(oldNick, newNick, false)
			}
		}
	}
}

// handleQuit handles QUIT messages
func (cm *ConnectionManager) handleQuit(msg *irc.Message) {
	nick := msg.Name
	hostmask := msg.User + "@" + msg.Host
	reason := msg.Trailing()

	// Log the quit event
	if cm.db != nil {
		content := fmt.Sprintf("%s has quit", nick)
		if reason != "" {
			content = fmt.Sprintf("%s has quit (%s)", nick, reason)
		}
		if err := cm.db.LogEvent(database.EventTypeQuit, "", nick, hostmask, content); err != nil {
			cm.logger.Warning("Failed to log quit event: %v", err)
		}
	}

	// Notify netsplit detector of the quit (for netsplit detection)
	cm.netsplitDetector.OnQuit(nick)

	// Notify tracker (user quit from all channels)
	if cm.channelUserTracker != nil {
		cm.channelUserTracker.OnQuit(nick)
	}

	// If the user who quit had our primary nickname, try to reclaim it
	if nick == cm.config.Server.Nickname && cm.currentNick != cm.config.Server.Nickname {
		cm.attemptPrimaryNickReclaim()
	}
}

// attemptPrimaryNickReclaim attempts to reclaim the primary nickname
func (cm *ConnectionManager) attemptPrimaryNickReclaim() {
	// Only attempt if we're not already using the primary nickname
	if cm.currentNick == cm.config.Server.Nickname {
		return
	}

	// Only attempt if we're registered and connected
	if !cm.registered || !cm.client.IsConnected() {
		return
	}

	cm.logger.Info("Primary nickname %s may be available, attempting to reclaim...", cm.config.Server.Nickname)
	cm.primaryNickReclaim = true

	err := cm.client.Write(&irc.Message{
		Command: "NICK",
		Params:  []string{cm.config.Server.Nickname},
	})
	if err != nil {
		cm.logger.Error("Failed to reclaim primary nickname: %v", err)
		cm.primaryNickReclaim = false
	}
}

// handleJoin handles JOIN messages
func (cm *ConnectionManager) handleJoin(msg *irc.Message) {
	if len(msg.Params) > 0 {
		channel := msg.Params[0]
		nick := msg.Name
		hostmask := msg.User + "@" + msg.Host

		if nick == cm.currentNick {
			// Notify channel manager
			cm.channelManager.OnJoin(channel)

			// Reset kick backoff on successful join
			// This handles both initial joins and rejoins after kicks
			cm.kickManager.ResetBackoff(channel)

			// Notify tracker (bot joined)
			if cm.channelUserTracker != nil {
				cm.channelUserTracker.OnJoin(channel, nick, true)
			}
		} else {
			// Log the join event
			if cm.db != nil {
				content := fmt.Sprintf("%s has joined %s", nick, channel)
				if err := cm.db.LogEvent(database.EventTypeJoin, channel, nick, hostmask, content); err != nil {
					cm.logger.Warning("Failed to log join event: %v", err)
				}
			}

			// Notify netsplit detector of the join (for netsplit recovery detection)
			cm.netsplitDetector.OnJoin(nick)
			cm.logger.Info("%s joined %s", nick, channel)

			// Notify tracker (other user joined)
			if cm.channelUserTracker != nil {
				cm.channelUserTracker.OnJoin(channel, nick, false)
			}
		}
	}
}

// handlePart handles PART messages
func (cm *ConnectionManager) handlePart(msg *irc.Message) {
	if len(msg.Params) > 0 {
		channel := msg.Params[0]
		nick := msg.Name
		hostmask := msg.User + "@" + msg.Host
		reason := msg.Trailing()

		if nick == cm.currentNick {
			// Notify channel manager
			cm.channelManager.OnPart(channel)

			// Notify tracker (bot left)
			if cm.channelUserTracker != nil {
				cm.channelUserTracker.OnPart(channel, nick, true)
			}
		} else {
			// Log the part event
			if cm.db != nil {
				content := fmt.Sprintf("%s has left %s", nick, channel)
				if reason != "" {
					content = fmt.Sprintf("%s has left %s (%s)", nick, channel, reason)
				}
				if err := cm.db.LogEvent(database.EventTypePart, channel, nick, hostmask, content); err != nil {
					cm.logger.Warning("Failed to log part event: %v", err)
				}
			}

			cm.logger.Info("%s left %s", nick, channel)

			// Notify tracker (other user left)
			if cm.channelUserTracker != nil {
				cm.channelUserTracker.OnPart(channel, nick, false)
			}
		}
	}
}

// handleKick handles KICK messages
func (cm *ConnectionManager) handleKick(msg *irc.Message) {
	if len(msg.Params) >= 2 {
		channel := msg.Params[0]
		kicked := msg.Params[1]
		reason := msg.Trailing()
		kicker := msg.Name
		kickerHostmask := msg.User + "@" + msg.Host

		// Log the kick event (for both bot and other users)
		if cm.db != nil {
			content := fmt.Sprintf("%s was kicked from %s by %s", kicked, channel, kicker)
			if reason != "" {
				content = fmt.Sprintf("%s was kicked from %s by %s (%s)", kicked, channel, kicker, reason)
			}
			if err := cm.db.LogEvent(database.EventTypeKick, channel, kicked, kickerHostmask, content); err != nil {
				cm.logger.Warning("Failed to log kick event: %v", err)
			}
		}

		if kicked == cm.currentNick {
			// Notify channel manager
			cm.channelManager.OnKick(channel)

			// Notify kick manager
			cm.kickManager.OnKick(channel, reason)

			// Notify tracker (bot was kicked)
			if cm.channelUserTracker != nil {
				cm.channelUserTracker.OnKick(channel, kicked, true)
			}

			// Schedule rejoin attempt
			go cm.handleKickRejoin(channel)
		} else {
			// Notify tracker (other user was kicked)
			if cm.channelUserTracker != nil {
				cm.channelUserTracker.OnKick(channel, kicked, false)
			}
		}
	}
}

// handlePing handles PING messages
func (cm *ConnectionManager) handlePing(msg *irc.Message) {
	if len(msg.Params) > 0 {
		cm.lastPingTime = time.Now()
		err := cm.client.Write(&irc.Message{
			Command: "PONG",
			Params:  []string{msg.Params[0]},
		})
		if err != nil {
			cm.logger.Error("Failed to send PONG: %v", err)
		}
	}
}

// handlePong handles PONG messages
func (cm *ConnectionManager) handlePong(_ *irc.Message) {
	cm.lastPongTime = time.Now()
}

// StartPingMonitor starts monitoring for ping timeouts
func (cm *ConnectionManager) StartPingMonitor() {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-cm.client.Context().Done():
				return
			case <-ticker.C:
				// Check if we've received a PONG recently
				// If no PONG for 90 seconds, consider it a timeout
				if time.Since(cm.lastPongTime) > 90*time.Second && cm.registered {
					cm.logger.Error("Ping timeout detected (no PONG for %v)", time.Since(cm.lastPongTime))
					cm.reconnectManager.OnPingTimeout()
					return
				}
			}
		}
	}()
}

// handlePrivMsg handles PRIVMSG messages
func (cm *ConnectionManager) handlePrivMsg(msg *irc.Message) {
	if len(msg.Params) >= 2 {
		// Check if this is a CTCP message first
		handled, strippedMessage := cm.ctcpHandler.HandleCTCP(msg)
		if handled {
			// CTCP message was fully handled (VERSION, PING, TIME, etc.)
			// Don't process as regular message
			return
		}

		target := msg.Params[0]
		message := msg.Trailing()

		// If we got a stripped message (ACTION), use that instead
		if strippedMessage != "" {
			message = strippedMessage
		}

		nick := msg.Name
		hostmask := msg.User + "@" + msg.Host

		isPM := target == cm.currentNick

		// Don't log here - let the message handler do it to avoid duplicates
		// The privMsgHandler callback will trigger HandleMessage which logs

		// Call the PRIVMSG handler callback if set
		if cm.privMsgHandler != nil {
			cm.privMsgHandler(nick, hostmask, target, message, isPM)
		}
	}
}

// handleNotice handles NOTICE messages
func (cm *ConnectionManager) handleNotice(msg *irc.Message) {
	if len(msg.Params) >= 2 {
		from := msg.Name
		message := msg.Trailing()

		// Check if this is a CTCP response
		if IsCTCPMessage(message) {
			ctcpCmd, ctcpArgs, ok := ParseCTCPMessage(message)
			if ok && cm.ctcpResponseHandler != nil {
				cm.ctcpResponseHandler(from, ctcpCmd, ctcpArgs)
				return
			}
		}

		cm.logger.Info("Notice from %s: %s", from, message)

		// Route to callback server if set (for IRC command tool responses)
		if cm.noticeHandler != nil {
			cm.noticeHandler(from, message)
		}
	}
}

// Run starts the IRC client event loop
func (cm *ConnectionManager) Run() error {
	return cm.client.Run()
}

// GetCurrentNick returns the current nickname being used
func (cm *ConnectionManager) GetCurrentNick() string {
	return cm.currentNick
}

// handleKickRejoin handles rejoining a channel after being kicked
func (cm *ConnectionManager) handleKickRejoin(channel string) {
	// Schedule the rejoin with appropriate delay
	shouldRejoin := <-cm.kickManager.ScheduleRejoin(channel)

	if !shouldRejoin {
		cm.logger.Warning("Not rejoining %s (banned)", channel)
		return
	}

	// Check if we're still connected
	if !cm.IsConnected() {
		cm.logger.Warning("Not rejoining %s (not connected)", channel)
		return
	}

	// Attempt to rejoin
	cm.logger.Info("Attempting to rejoin %s", channel)
	err := cm.client.JoinChannel(channel)
	if err != nil {
		cm.logger.Error("Failed to rejoin %s: %v", channel, err)
		// Increase backoff for next attempt
		cm.kickManager.IncreaseBackoff(channel)
		// Schedule another rejoin attempt
		go cm.handleKickRejoin(channel)
		return
	}

	// Note: We'll reset the backoff when we receive the JOIN confirmation
	// in handleJoin, not here, to ensure we actually successfully joined
}

// handleBan detects and handles ban events
func (cm *ConnectionManager) handleBan(channel string) {
	cm.kickManager.OnBan(channel)
	cm.channelManager.OnKick(channel)
}

// handleBanError handles ERR_BANNEDFROMCHAN (474) messages
func (cm *ConnectionManager) handleBanError(msg *irc.Message) {
	// Format: :server 474 <nick> <channel> :Cannot join channel (+b)
	if len(msg.Params) >= 2 {
		channel := msg.Params[1]
		cm.handleBan(channel)
	}
}

// GetKickManager returns the kick manager
func (cm *ConnectionManager) GetKickManager() *KickManager {
	return cm.kickManager
}

// GetChannelManager returns the channel manager
func (cm *ConnectionManager) GetChannelManager() *ChannelManager {
	return cm.channelManager
}

// GetNetsplitDetector returns the netsplit detector
func (cm *ConnectionManager) GetNetsplitDetector() *NetsplitDetector {
	return cm.netsplitDetector
}

// SetPrivMsgHandler sets a callback function to handle PRIVMSG events
// This allows the bot's message handler to process incoming messages
func (cm *ConnectionManager) SetPrivMsgHandler(handler PrivMsgHandler) {
	cm.privMsgHandler = handler
}

// SetNoticeHandler sets a callback function to handle NOTICE events
// This is used by the callback server to collect service responses
func (cm *ConnectionManager) SetNoticeHandler(handler NoticeHandler) {
	cm.noticeHandler = handler
}

// SetNumericHandler sets a callback function to handle numeric IRC responses
// This is used by the callback server to collect WHOIS/WHOWAS responses
func (cm *ConnectionManager) SetNumericHandler(handler NumericHandler) {
	cm.numericHandler = handler
}

// handleNumericIfApplicable checks if a message is a numeric response and routes it
func (cm *ConnectionManager) handleNumericIfApplicable(msg *irc.Message) {
	// Try to parse command as numeric
	if len(msg.Command) == 3 {
		var numeric int
		_, err := fmt.Sscanf(msg.Command, "%d", &numeric)
		if err == nil {
			// Handle specific numerics for channel tracking
			switch numeric {
			case 353: // RPL_NAMREPLY - Names list for a channel
				// Format: :server 353 <nick> <type> <channel> :<names>
				// type is = (public), * (private), @ (secret)
				if len(msg.Params) >= 3 {
					channel := msg.Params[2]
					names := strings.Fields(msg.Trailing())
					if cm.channelUserTracker != nil {
						cm.channelUserTracker.OnNamesReply(channel, names)
					}
				}

			case 366: // RPL_ENDOFNAMES - End of names list
				// We don't need to do anything special here, names are already processed

			case 332: // RPL_TOPIC - Channel topic
				// Format: :server 332 <nick> <channel> :<topic>
				if len(msg.Params) >= 2 {
					channel := msg.Params[1]
					topic := msg.Trailing()
					if cm.channelUserTracker != nil {
						cm.channelUserTracker.OnTopic(channel, topic)
					}
				}
			}

			// Route to callback server if set
			if cm.numericHandler != nil {
				cm.numericHandler(numeric, msg.Params)
			}
		}
	}
}

// SetCTCPResponseHandler sets a callback function to handle CTCP responses
// This is used by the callback server to collect VERSION/TIME responses
func (cm *ConnectionManager) SetCTCPResponseHandler(handler CTCPResponseHandler) {
	cm.ctcpResponseHandler = handler
}

// SetChannelUserTracker sets the channel user tracker for tracking user state
func (cm *ConnectionManager) SetChannelUserTracker(tracker ChannelUserTracker) {
	cm.channelUserTracker = tracker
}

// handleMode handles MODE messages for channel user tracking
func (cm *ConnectionManager) handleMode(msg *irc.Message) {
	// MODE format: :nick!user@host MODE #channel +o target
	// or: :nick!user@host MODE #channel +v-o target1 target2
	if len(msg.Params) < 2 {
		return
	}

	target := msg.Params[0]

	// Only handle channel modes (not user modes)
	if !strings.HasPrefix(target, "#") && !strings.HasPrefix(target, "&") {
		return
	}

	channel := target
	modeString := msg.Params[1]
	modeArgs := msg.Params[2:] // Targets for the modes
	setter := msg.Name
	setterHostmask := msg.User + "@" + msg.Host

	// Parse mode changes
	adding := true
	argIndex := 0

	for _, char := range modeString {
		switch char {
		case '+':
			adding = true
		case '-':
			adding = false
		case 'o', 'h', 'v': // Op, halfop, voice - these take a nick argument
			if argIndex < len(modeArgs) {
				nick := modeArgs[argIndex]
				argIndex++

				// Check if this affects the bot
				isSelf := strings.EqualFold(nick, cm.currentNick)

				// Log the mode change event
				if cm.db != nil {
					modeChar := string(char)
					var content string
					if adding {
						content = fmt.Sprintf("%s sets mode +%s on %s", setter, modeChar, nick)
					} else {
						content = fmt.Sprintf("%s sets mode -%s on %s", setter, modeChar, nick)
					}
					if err := cm.db.LogEvent(database.EventTypeMode, channel, nick, setterHostmask, content); err != nil {
						cm.logger.Warning("Failed to log mode event: %v", err)
					}
				}

				// Notify tracker
				if cm.channelUserTracker != nil {
					cm.channelUserTracker.OnMode(channel, nick, string(char), adding, isSelf)
				}

				// Log the mode change
				modeChar := string(char)
				if adding {
					cm.logger.Info("Mode +%s set on %s in %s", modeChar, nick, channel)
				} else {
					cm.logger.Info("Mode -%s removed from %s in %s", modeChar, nick, channel)
				}
			}
		case 'b': // Ban - log ban/unban events
			if argIndex < len(modeArgs) {
				mask := modeArgs[argIndex]
				argIndex++

				// Log ban/unban events
				if cm.db != nil {
					var content string
					var eventType string
					if adding {
						content = fmt.Sprintf("%s sets ban +b %s", setter, mask)
						eventType = database.EventTypeBan
					} else {
						content = fmt.Sprintf("%s removes ban -b %s", setter, mask)
						eventType = database.EventTypeUnban
					}
					if err := cm.db.LogEvent(eventType, channel, mask, setterHostmask, content); err != nil {
						cm.logger.Warning("Failed to log ban event: %v", err)
					}
				}
			}
		case 'q', 'e', 'I': // Quiet, exempt, invite - these take a mask argument
			if argIndex < len(modeArgs) {
				argIndex++ // Skip the mask, we don't track these
			}
		case 'k': // Key - takes argument when adding
			if adding && argIndex < len(modeArgs) {
				argIndex++
			}
		case 'l': // Limit - takes argument when adding
			if adding && argIndex < len(modeArgs) {
				argIndex++
			}
			// Other modes (n, t, s, i, etc.) don't take arguments
		}
	}
}

// handleTopicChange handles TOPIC messages
func (cm *ConnectionManager) handleTopicChange(msg *irc.Message) {
	// TOPIC format: :nick!user@host TOPIC #channel :new topic
	if len(msg.Params) < 1 {
		return
	}

	channel := msg.Params[0]
	topic := msg.Trailing()
	setter := msg.Name
	setterHostmask := msg.User + "@" + msg.Host

	cm.logger.Info("Topic in %s changed to: %s", channel, topic)

	// Log the topic change event
	if cm.db != nil {
		content := fmt.Sprintf("%s changed topic to: %s", setter, topic)
		if err := cm.db.LogEvent(database.EventTypeTopic, channel, setter, setterHostmask, content); err != nil {
			cm.logger.Warning("Failed to log topic event: %v", err)
		}
	}

	// Notify tracker
	if cm.channelUserTracker != nil {
		cm.channelUserTracker.OnTopic(channel, topic)
	}
}
