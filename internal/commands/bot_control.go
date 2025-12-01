package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/yourusername/lolo/internal/database"
	"gopkg.in/irc.v4"
)

// PMEnableCommand implements the !pm enable command
type PMEnableCommand struct {
	db *database.DB
}

// NewPMEnableCommand creates a new PM enable command
func NewPMEnableCommand(db *database.DB) *PMEnableCommand {
	return &PMEnableCommand{
		db: db,
	}
}

// Name returns the command name
func (c *PMEnableCommand) Name() string {
	return "pm enable"
}

// Execute runs the PM enable command
func (c *PMEnableCommand) Execute(ctx *Context) (*Response, error) {
	// Set PM state to enabled (Requirement 18.3)
	err := c.db.SetPMState(true)
	if err != nil {
		return nil, fmt.Errorf("failed to enable PMs: %w", err)
	}

	// Log audit action (Requirement 29.3)
	auditErr := c.db.LogAuditAction(
		ctx.Nick,
		ctx.Hostmask,
		"pm_enable",
		"",
		"",
		"success",
	)
	if auditErr != nil {
		fmt.Printf("Warning: failed to log audit action: %v\n", auditErr)
	}

	return NewResponse("Private messages enabled."), nil
}

// RequiredPermission returns the minimum permission level needed
// Only owner can enable/disable PMs (Requirement 18.3)
func (c *PMEnableCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelOwner
}

// Help returns help text for this command
func (c *PMEnableCommand) Help() string {
	return "!pm enable - Enable private message responses (owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *PMEnableCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for PM enable command
}

// PMDisableCommand implements the !pm disable command
type PMDisableCommand struct {
	db *database.DB
}

// NewPMDisableCommand creates a new PM disable command
func NewPMDisableCommand(db *database.DB) *PMDisableCommand {
	return &PMDisableCommand{
		db: db,
	}
}

// Name returns the command name
func (c *PMDisableCommand) Name() string {
	return "pm disable"
}

// Execute runs the PM disable command
func (c *PMDisableCommand) Execute(ctx *Context) (*Response, error) {
	// Set PM state to disabled (Requirement 18.3)
	err := c.db.SetPMState(false)
	if err != nil {
		return nil, fmt.Errorf("failed to disable PMs: %w", err)
	}

	// Log audit action (Requirement 29.3)
	auditErr := c.db.LogAuditAction(
		ctx.Nick,
		ctx.Hostmask,
		"pm_disable",
		"",
		"",
		"success",
	)
	if auditErr != nil {
		fmt.Printf("Warning: failed to log audit action: %v\n", auditErr)
	}

	return NewResponse("Private messages disabled. Only owner PMs will be processed."), nil
}

// RequiredPermission returns the minimum permission level needed
// Only owner can enable/disable PMs (Requirement 18.3)
func (c *PMDisableCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelOwner
}

// Help returns help text for this command
func (c *PMDisableCommand) Help() string {
	return "!pm disable - Disable private message responses except from owner (owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *PMDisableCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for PM disable command
}

// ChannelEnableCommand implements the !channel enable command
type ChannelEnableCommand struct {
	db *database.DB
}

// NewChannelEnableCommand creates a new channel enable command
func NewChannelEnableCommand(db *database.DB) *ChannelEnableCommand {
	return &ChannelEnableCommand{
		db: db,
	}
}

// Name returns the command name
func (c *ChannelEnableCommand) Name() string {
	return "channel enable"
}

// Execute runs the channel enable command
func (c *ChannelEnableCommand) Execute(ctx *Context) (*Response, error) {
	// Check arguments: !channel enable <channel>
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !channel enable <channel>"), nil
	}

	channel := ctx.Args[0]

	// Ensure channel name starts with #
	if !strings.HasPrefix(channel, "#") {
		channel = "#" + channel
	}

	// Set channel state to enabled (Requirement 18.4, 20.4)
	err := c.db.SetChannelState(channel, true)
	if err != nil {
		return nil, fmt.Errorf("failed to enable channel: %w", err)
	}

	// Log audit action (Requirement 29.2)
	auditErr := c.db.LogAuditAction(
		ctx.Nick,
		ctx.Hostmask,
		"channel_enable",
		"",
		fmt.Sprintf("channel=%s", channel),
		"success",
	)
	if auditErr != nil {
		fmt.Printf("Warning: failed to log audit action: %v\n", auditErr)
	}

	return NewResponse(fmt.Sprintf("Channel %s enabled.", channel)), nil
}

// RequiredPermission returns the minimum permission level needed
// Only owner can enable/disable channels (Requirement 18.4)
func (c *ChannelEnableCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelOwner
}

// Help returns help text for this command
func (c *ChannelEnableCommand) Help() string {
	return "!channel enable <channel> - Enable bot responses in a channel (owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *ChannelEnableCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for channel enable command
}

// ChannelDisableCommand implements the !channel disable command
type ChannelDisableCommand struct {
	db *database.DB
}

// NewChannelDisableCommand creates a new channel disable command
func NewChannelDisableCommand(db *database.DB) *ChannelDisableCommand {
	return &ChannelDisableCommand{
		db: db,
	}
}

// Name returns the command name
func (c *ChannelDisableCommand) Name() string {
	return "channel disable"
}

// Execute runs the channel disable command
func (c *ChannelDisableCommand) Execute(ctx *Context) (*Response, error) {
	// Check arguments: !channel disable <channel>
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !channel disable <channel>"), nil
	}

	channel := ctx.Args[0]

	// Ensure channel name starts with #
	if !strings.HasPrefix(channel, "#") {
		channel = "#" + channel
	}

	// Set channel state to disabled (Requirement 18.4, 20.2)
	err := c.db.SetChannelState(channel, false)
	if err != nil {
		return nil, fmt.Errorf("failed to disable channel: %w", err)
	}

	// Log audit action (Requirement 29.2)
	auditErr := c.db.LogAuditAction(
		ctx.Nick,
		ctx.Hostmask,
		"channel_disable",
		"",
		fmt.Sprintf("channel=%s", channel),
		"success",
	)
	if auditErr != nil {
		fmt.Printf("Warning: failed to log audit action: %v\n", auditErr)
	}

	return NewResponse(fmt.Sprintf("Channel %s disabled. Bot will not process commands or mentions from this channel.", channel)), nil
}

// RequiredPermission returns the minimum permission level needed
// Only owner can enable/disable channels (Requirement 18.4)
func (c *ChannelDisableCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelOwner
}

// Help returns help text for this command
func (c *ChannelDisableCommand) Help() string {
	return "!channel disable <channel> - Disable bot responses in a channel (owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *ChannelDisableCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for channel disable command
}

// JoinCommand implements the !join command
// Requirement 19.3: Support join operation for owner
type JoinCommand struct {
	client IRCClient
}

// NewJoinCommand creates a new join command
func NewJoinCommand(client IRCClient) *JoinCommand {
	return &JoinCommand{
		client: client,
	}
}

// Name returns the command name
func (c *JoinCommand) Name() string {
	return "join"
}

// Execute runs the join command
func (c *JoinCommand) Execute(ctx *Context) (*Response, error) {
	// Check arguments: !join <channel>
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !join <channel>"), nil
	}

	channel := ctx.Args[0]

	// Ensure channel name starts with #
	if !strings.HasPrefix(channel, "#") {
		channel = "#" + channel
	}

	// Send JOIN command to IRC server
	err := c.client.Write(&irc.Message{
		Command: "JOIN",
		Params:  []string{channel},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to join channel: %w", err)
	}

	return NewResponse(fmt.Sprintf("Joining %s", channel)), nil
}

// RequiredPermission returns the minimum permission level needed
// Requirement 19.3: Admin or owner can use bot control commands
func (c *JoinCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for this command
func (c *JoinCommand) Help() string {
	return "!join <channel> - Join a channel (admin/owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *JoinCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for join command
}

// PartCommand implements the !part command
// Requirement 19.3: Support part operation for owner
type PartCommand struct {
	client IRCClient
}

// NewPartCommand creates a new part command
func NewPartCommand(client IRCClient) *PartCommand {
	return &PartCommand{
		client: client,
	}
}

// Name returns the command name
func (c *PartCommand) Name() string {
	return "part"
}

// Execute runs the part command
func (c *PartCommand) Execute(ctx *Context) (*Response, error) {
	// Check arguments: !part <channel> [message]
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !part <channel> [message]"), nil
	}

	channel := ctx.Args[0]

	// Ensure channel name starts with #
	if !strings.HasPrefix(channel, "#") {
		channel = "#" + channel
	}

	// Build part message
	params := []string{channel}
	if len(ctx.Args) > 1 {
		message := strings.Join(ctx.Args[1:], " ")
		params = append(params, message)
	}

	// Send PART command to IRC server
	err := c.client.Write(&irc.Message{
		Command: "PART",
		Params:  params,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to part channel: %w", err)
	}

	return NewResponse(fmt.Sprintf("Leaving %s", channel)), nil
}

// RequiredPermission returns the minimum permission level needed
// Requirement 19.3: Admin or owner can use bot control commands
func (c *PartCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for this command
func (c *PartCommand) Help() string {
	return "!part <channel> [message] - Leave a channel (admin/owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *PartCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for part command
}

// NickCommand implements the !nick command
// Requirement 19.3: Support nick operation for owner
type NickCommand struct {
	client IRCClient
}

// NewNickCommand creates a new nick command
func NewNickCommand(client IRCClient) *NickCommand {
	return &NickCommand{
		client: client,
	}
}

// Name returns the command name
func (c *NickCommand) Name() string {
	return "nick"
}

// Execute runs the nick command
func (c *NickCommand) Execute(ctx *Context) (*Response, error) {
	// Check arguments: !nick <newnick>
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !nick <newnick>"), nil
	}

	newNick := ctx.Args[0]

	// Send NICK command to IRC server
	err := c.client.Write(&irc.Message{
		Command: "NICK",
		Params:  []string{newNick},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to change nickname: %w", err)
	}

	return NewResponse(fmt.Sprintf("Changing nickname to %s", newNick)), nil
}

// RequiredPermission returns the minimum permission level needed
// Requirement 19.3: Only owner can use bot control commands
func (c *NickCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelOwner
}

// Help returns help text for this command
func (c *NickCommand) Help() string {
	return "!nick <newnick> - Change bot nickname (owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *NickCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for nick command
}

// QuitCommand implements the !quit command
// Requirement 19.3: Support quit operation for owner
type QuitCommand struct {
	client IRCClient
}

// NewQuitCommand creates a new quit command
func NewQuitCommand(client IRCClient) *QuitCommand {
	return &QuitCommand{
		client: client,
	}
}

// Name returns the command name
func (c *QuitCommand) Name() string {
	return "quit"
}

// Execute runs the quit command
func (c *QuitCommand) Execute(ctx *Context) (*Response, error) {
	// Build quit message
	message := "Quit command received"
	if len(ctx.Args) > 0 {
		message = strings.Join(ctx.Args, " ")
	}

	// Send QUIT command to IRC server
	err := c.client.Write(&irc.Message{
		Command: "QUIT",
		Params:  []string{message},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to quit: %w", err)
	}

	return NewResponse("Shutting down..."), nil
}

// RequiredPermission returns the minimum permission level needed
// Requirement 19.3: Only owner can use bot control commands
func (c *QuitCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelOwner
}

// Help returns help text for this command
func (c *QuitCommand) Help() string {
	return "!quit [message] - Disconnect from IRC and shut down (owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *QuitCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for quit command
}
