package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/yourusername/lolo/internal/database"
	"gopkg.in/irc.v4"
)

// IRCClient defines the interface for sending IRC commands
// This allows commands to interact with the IRC server
type IRCClient interface {
	Write(msg *irc.Message) error
	IsConnected() bool
}

// KickCommand implements the !kick command
// Requirement 19.1: Support kick operation for admin/owner
type KickCommand struct {
	client IRCClient
}

// NewKickCommand creates a new kick command
func NewKickCommand(client IRCClient) *KickCommand {
	return &KickCommand{
		client: client,
	}
}

// Name returns the command name
func (c *KickCommand) Name() string {
	return "kick"
}

// Execute runs the kick command
// Requirement 19.8: Channel operator status verification
// Note: The IRC server will reject the command if the bot lacks operator status.
// The bot does not track its own channel modes, so verification happens server-side.
func (c *KickCommand) Execute(ctx *Context) (*Response, error) {
	// Must be used in a channel, not PM
	if ctx.IsPM {
		return NewErrorResponse("This command can only be used in a channel."), nil
	}

	// Check arguments: !kick <nick> [reason]
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !kick <nick> [reason]"), nil
	}

	nick := ctx.Args[0]
	reason := "Kicked by " + ctx.Nick
	if len(ctx.Args) > 1 {
		reason = strings.Join(ctx.Args[1:], " ")
	}

	// Send KICK command to IRC server
	err := c.client.Write(&irc.Message{
		Command: "KICK",
		Params:  []string{ctx.Channel, nick, reason},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to kick user: %w", err)
	}

	return NewResponse(fmt.Sprintf("Kicked %s from %s", nick, ctx.Channel)), nil
}

// RequiredPermission returns the minimum permission level needed
// Requirement 19.1: Admin or owner can use moderation commands
func (c *KickCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for this command
func (c *KickCommand) Help() string {
	return "!kick <nick> [reason] - Kick a user from the channel (admin/owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *KickCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for kick command
}

// BanCommand implements the !ban command
// Requirement 19.1: Support ban operation for admin/owner
type BanCommand struct {
	client IRCClient
}

// NewBanCommand creates a new ban command
func NewBanCommand(client IRCClient) *BanCommand {
	return &BanCommand{
		client: client,
	}
}

// Name returns the command name
func (c *BanCommand) Name() string {
	return "ban"
}

// Execute runs the ban command
// Requirement 19.8: Channel operator status verification
// Note: The IRC server will reject the command if the bot lacks operator status.
func (c *BanCommand) Execute(ctx *Context) (*Response, error) {
	// Must be used in a channel, not PM
	if ctx.IsPM {
		return NewErrorResponse("This command can only be used in a channel."), nil
	}

	// Check arguments: !ban <hostmask>
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !ban <hostmask>"), nil
	}

	hostmask := ctx.Args[0]

	// Send MODE +b command to IRC server
	err := c.client.Write(&irc.Message{
		Command: "MODE",
		Params:  []string{ctx.Channel, "+b", hostmask},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to ban user: %w", err)
	}

	return NewResponse(fmt.Sprintf("Banned %s from %s", hostmask, ctx.Channel)), nil
}

// RequiredPermission returns the minimum permission level needed
// Requirement 19.1: Admin or owner can use moderation commands
func (c *BanCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for this command
func (c *BanCommand) Help() string {
	return "!ban <hostmask> - Ban a user from the channel (admin/owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *BanCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for ban command
}

// UnbanCommand implements the !unban command
// Requirement 19.1: Support unban operation for admin/owner
type UnbanCommand struct {
	client IRCClient
}

// NewUnbanCommand creates a new unban command
func NewUnbanCommand(client IRCClient) *UnbanCommand {
	return &UnbanCommand{
		client: client,
	}
}

// Name returns the command name
func (c *UnbanCommand) Name() string {
	return "unban"
}

// Execute runs the unban command
// Requirement 19.8: Channel operator status verification
// Note: The IRC server will reject the command if the bot lacks operator status.
func (c *UnbanCommand) Execute(ctx *Context) (*Response, error) {
	// Must be used in a channel, not PM
	if ctx.IsPM {
		return NewErrorResponse("This command can only be used in a channel."), nil
	}

	// Check arguments: !unban <hostmask>
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !unban <hostmask>"), nil
	}

	hostmask := ctx.Args[0]

	// Send MODE -b command to IRC server
	err := c.client.Write(&irc.Message{
		Command: "MODE",
		Params:  []string{ctx.Channel, "-b", hostmask},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to unban user: %w", err)
	}

	return NewResponse(fmt.Sprintf("Unbanned %s from %s", hostmask, ctx.Channel)), nil
}

// RequiredPermission returns the minimum permission level needed
// Requirement 19.1: Admin or owner can use moderation commands
func (c *UnbanCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for this command
func (c *UnbanCommand) Help() string {
	return "!unban <hostmask> - Unban a user from the channel (admin/owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *UnbanCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for unban command
}

// KickBanCommand implements the !kickban command
// Requirement 19.1: Support kickban operation for admin/owner
type KickBanCommand struct {
	client IRCClient
}

// NewKickBanCommand creates a new kickban command
func NewKickBanCommand(client IRCClient) *KickBanCommand {
	return &KickBanCommand{
		client: client,
	}
}

// Name returns the command name
func (c *KickBanCommand) Name() string {
	return "kickban"
}

// Execute runs the kickban command
// Requirement 19.8: Channel operator status verification
// Note: The IRC server will reject the command if the bot lacks operator status.
func (c *KickBanCommand) Execute(ctx *Context) (*Response, error) {
	// Must be used in a channel, not PM
	if ctx.IsPM {
		return NewErrorResponse("This command can only be used in a channel."), nil
	}

	// Check arguments: !kickban <nick> [reason]
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !kickban <nick> [reason]"), nil
	}

	nick := ctx.Args[0]
	reason := "Kicked and banned by " + ctx.Nick
	if len(ctx.Args) > 1 {
		reason = strings.Join(ctx.Args[1:], " ")
	}

	// First, ban the user (using *!*@host pattern)
	// In a real implementation, we'd need to get the user's hostmask first
	// For now, we'll use a simple nick!*@* pattern
	hostmask := nick + "!*@*"

	err := c.client.Write(&irc.Message{
		Command: "MODE",
		Params:  []string{ctx.Channel, "+b", hostmask},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to ban user: %w", err)
	}

	// Then kick the user
	err = c.client.Write(&irc.Message{
		Command: "KICK",
		Params:  []string{ctx.Channel, nick, reason},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to kick user: %w", err)
	}

	return NewResponse(fmt.Sprintf("Kicked and banned %s from %s", nick, ctx.Channel)), nil
}

// RequiredPermission returns the minimum permission level needed
// Requirement 19.1: Admin or owner can use moderation commands
func (c *KickBanCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for this command
func (c *KickBanCommand) Help() string {
	return "!kickban <nick> [reason] - Kick and ban a user from the channel (admin/owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *KickBanCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for kickban command
}

// MuteCommand implements the !mute command
// Requirement 19.1: Support mute operation for admin/owner
type MuteCommand struct {
	client IRCClient
}

// NewMuteCommand creates a new mute command
func NewMuteCommand(client IRCClient) *MuteCommand {
	return &MuteCommand{
		client: client,
	}
}

// Name returns the command name
func (c *MuteCommand) Name() string {
	return "mute"
}

// Execute runs the mute command
// Requirement 19.8: Channel operator status verification
// Note: The IRC server will reject the command if the bot lacks operator status.
func (c *MuteCommand) Execute(ctx *Context) (*Response, error) {
	// Must be used in a channel, not PM
	if ctx.IsPM {
		return NewErrorResponse("This command can only be used in a channel."), nil
	}

	// Check arguments: !mute <nick>
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !mute <nick>"), nil
	}

	nick := ctx.Args[0]

	// Mute by setting +q (quiet) mode on the user
	// Using nick!*@* pattern for the quiet mask
	hostmask := nick + "!*@*"

	err := c.client.Write(&irc.Message{
		Command: "MODE",
		Params:  []string{ctx.Channel, "+q", hostmask},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to mute user: %w", err)
	}

	return NewResponse(fmt.Sprintf("Muted %s in %s", nick, ctx.Channel)), nil
}

// RequiredPermission returns the minimum permission level needed
// Requirement 19.1: Admin or owner can use moderation commands
func (c *MuteCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for this command
func (c *MuteCommand) Help() string {
	return "!mute <nick> - Mute a user in the channel (admin/owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *MuteCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for mute command
}

// UnmuteCommand implements the !unmute command
// Requirement 19.1: Support unmute operation for admin/owner
type UnmuteCommand struct {
	client IRCClient
}

// NewUnmuteCommand creates a new unmute command
func NewUnmuteCommand(client IRCClient) *UnmuteCommand {
	return &UnmuteCommand{
		client: client,
	}
}

// Name returns the command name
func (c *UnmuteCommand) Name() string {
	return "unmute"
}

// Execute runs the unmute command
// Requirement 19.8: Channel operator status verification
// Note: The IRC server will reject the command if the bot lacks operator status.
func (c *UnmuteCommand) Execute(ctx *Context) (*Response, error) {
	// Must be used in a channel, not PM
	if ctx.IsPM {
		return NewErrorResponse("This command can only be used in a channel."), nil
	}

	// Check arguments: !unmute <nick>
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !unmute <nick>"), nil
	}

	nick := ctx.Args[0]

	// Unmute by removing +q (quiet) mode from the user
	// Using nick!*@* pattern for the quiet mask
	hostmask := nick + "!*@*"

	err := c.client.Write(&irc.Message{
		Command: "MODE",
		Params:  []string{ctx.Channel, "-q", hostmask},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to unmute user: %w", err)
	}

	return NewResponse(fmt.Sprintf("Unmuted %s in %s", nick, ctx.Channel)), nil
}

// RequiredPermission returns the minimum permission level needed
// Requirement 19.1: Admin or owner can use moderation commands
func (c *UnmuteCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for this command
func (c *UnmuteCommand) Help() string {
	return "!unmute <nick> - Unmute a user in the channel (admin/owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *UnmuteCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for unmute command
}
