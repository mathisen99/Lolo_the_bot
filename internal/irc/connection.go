package irc

import (
	"fmt"
	"time"

	"github.com/yourusername/lolo/internal/config"
	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/output"
	"github.com/yourusername/lolo/internal/user"
	"gopkg.in/irc.v4"
)

// PrivMsgHandler is a callback function for handling PRIVMSG events
type PrivMsgHandler func(nick, hostmask, channel, message string, isPM bool)

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

	lastPingTime     time.Time
	lastPongTime     time.Time
	reconnectManager *ReconnectionManager
	kickManager      *KickManager
	channelManager   *ChannelManager
	ctcpHandler      *CTCPHandler
	netsplitDetector *NetsplitDetector
	privMsgHandler   PrivMsgHandler // Callback for handling PRIVMSG events
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
		} else if oldNick == cm.config.Server.Nickname && newNick != cm.currentNick {
			// Someone else changed from our primary nickname - try to reclaim it
			cm.attemptPrimaryNickReclaim()
		}
	}
}

// handleQuit handles QUIT messages
func (cm *ConnectionManager) handleQuit(msg *irc.Message) {
	nick := msg.Name

	// Notify netsplit detector of the quit (for netsplit detection)
	cm.netsplitDetector.OnQuit(nick)

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

		if nick == cm.currentNick {
			// Notify channel manager
			cm.channelManager.OnJoin(channel)

			// Reset kick backoff on successful join
			// This handles both initial joins and rejoins after kicks
			cm.kickManager.ResetBackoff(channel)
		} else {
			// Notify netsplit detector of the join (for netsplit recovery detection)
			cm.netsplitDetector.OnJoin(nick)
			cm.logger.Info("%s joined %s", nick, channel)
		}
	}
}

// handlePart handles PART messages
func (cm *ConnectionManager) handlePart(msg *irc.Message) {
	if len(msg.Params) > 0 {
		channel := msg.Params[0]
		nick := msg.Name

		if nick == cm.currentNick {
			// Notify channel manager
			cm.channelManager.OnPart(channel)
		} else {
			cm.logger.Info("%s left %s", nick, channel)
		}
	}
}

// handleKick handles KICK messages
func (cm *ConnectionManager) handleKick(msg *irc.Message) {
	if len(msg.Params) >= 2 {
		channel := msg.Params[0]
		kicked := msg.Params[1]
		reason := msg.Trailing()

		if kicked == cm.currentNick {
			// Notify channel manager
			cm.channelManager.OnKick(channel)

			// Notify kick manager
			cm.kickManager.OnKick(channel, reason)

			// Schedule rejoin attempt
			go cm.handleKickRejoin(channel)
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
		cm.logger.Info("Notice from %s: %s", from, message)
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
