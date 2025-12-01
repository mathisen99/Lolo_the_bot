package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/yourusername/lolo/internal/database"
	"gopkg.in/irc.v4"
)

// OpCommand implements the !op command
// Requirement 19.2: Support op operation for admin/owner
type OpCommand struct {
	client IRCClient
}

// NewOpCommand creates a new op command
func NewOpCommand(client IRCClient) *OpCommand {
	return &OpCommand{
		client: client,
	}
}

// Name returns the command name
func (c *OpCommand) Name() string {
	return "op"
}

// Execute runs the op command
// Requirement 19.8: Channel operator status verification
// Note: The IRC server will reject the command if the bot lacks operator status.
func (c *OpCommand) Execute(ctx *Context) (*Response, error) {
	// Must be used in a channel, not PM
	if ctx.IsPM {
		return NewErrorResponse("This command can only be used in a channel."), nil
	}

	// Check arguments: !op <nick>
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !op <nick>"), nil
	}

	nick := ctx.Args[0]

	// Send MODE +o command to IRC server
	err := c.client.Write(&irc.Message{
		Command: "MODE",
		Params:  []string{ctx.Channel, "+o", nick},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to op user: %w", err)
	}

	return NewResponse(fmt.Sprintf("Gave operator status to %s in %s", nick, ctx.Channel)), nil
}

// RequiredPermission returns the minimum permission level needed
// Requirement 19.2: Admin or owner can use channel management commands
func (c *OpCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for this command
func (c *OpCommand) Help() string {
	return "!op <nick> - Give operator status to a user (admin/owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *OpCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for op command
}

// DeopCommand implements the !deop command
// Requirement 19.2: Support deop operation for admin/owner
type DeopCommand struct {
	client IRCClient
}

// NewDeopCommand creates a new deop command
func NewDeopCommand(client IRCClient) *DeopCommand {
	return &DeopCommand{
		client: client,
	}
}

// Name returns the command name
func (c *DeopCommand) Name() string {
	return "deop"
}

// Execute runs the deop command
// Requirement 19.8: Channel operator status verification
// Note: The IRC server will reject the command if the bot lacks operator status.
func (c *DeopCommand) Execute(ctx *Context) (*Response, error) {
	// Must be used in a channel, not PM
	if ctx.IsPM {
		return NewErrorResponse("This command can only be used in a channel."), nil
	}

	// Check arguments: !deop <nick>
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !deop <nick>"), nil
	}

	nick := ctx.Args[0]

	// Send MODE -o command to IRC server
	err := c.client.Write(&irc.Message{
		Command: "MODE",
		Params:  []string{ctx.Channel, "-o", nick},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to deop user: %w", err)
	}

	return NewResponse(fmt.Sprintf("Removed operator status from %s in %s", nick, ctx.Channel)), nil
}

// RequiredPermission returns the minimum permission level needed
// Requirement 19.2: Admin or owner can use channel management commands
func (c *DeopCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for this command
func (c *DeopCommand) Help() string {
	return "!deop <nick> - Remove operator status from a user (admin/owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *DeopCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for deop command
}

// VoiceCommand implements the !voice command
// Requirement 19.2: Support voice operation for admin/owner
type VoiceCommand struct {
	client IRCClient
}

// NewVoiceCommand creates a new voice command
func NewVoiceCommand(client IRCClient) *VoiceCommand {
	return &VoiceCommand{
		client: client,
	}
}

// Name returns the command name
func (c *VoiceCommand) Name() string {
	return "voice"
}

// Execute runs the voice command
// Requirement 19.8: Channel operator status verification
// Note: The IRC server will reject the command if the bot lacks operator status.
func (c *VoiceCommand) Execute(ctx *Context) (*Response, error) {
	// Must be used in a channel, not PM
	if ctx.IsPM {
		return NewErrorResponse("This command can only be used in a channel."), nil
	}

	// Check arguments: !voice <nick>
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !voice <nick>"), nil
	}

	nick := ctx.Args[0]

	// Send MODE +v command to IRC server
	err := c.client.Write(&irc.Message{
		Command: "MODE",
		Params:  []string{ctx.Channel, "+v", nick},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to voice user: %w", err)
	}

	return NewResponse(fmt.Sprintf("Gave voice to %s in %s", nick, ctx.Channel)), nil
}

// RequiredPermission returns the minimum permission level needed
// Requirement 19.2: Admin or owner can use channel management commands
func (c *VoiceCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for this command
func (c *VoiceCommand) Help() string {
	return "!voice <nick> - Give voice to a user (admin/owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *VoiceCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for voice command
}

// DevoiceCommand implements the !devoice command
// Requirement 19.2: Support devoice operation for admin/owner
type DevoiceCommand struct {
	client IRCClient
}

// NewDevoiceCommand creates a new devoice command
func NewDevoiceCommand(client IRCClient) *DevoiceCommand {
	return &DevoiceCommand{
		client: client,
	}
}

// Name returns the command name
func (c *DevoiceCommand) Name() string {
	return "devoice"
}

// Execute runs the devoice command
// Requirement 19.8: Channel operator status verification
// Note: The IRC server will reject the command if the bot lacks operator status.
func (c *DevoiceCommand) Execute(ctx *Context) (*Response, error) {
	// Must be used in a channel, not PM
	if ctx.IsPM {
		return NewErrorResponse("This command can only be used in a channel."), nil
	}

	// Check arguments: !devoice <nick>
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !devoice <nick>"), nil
	}

	nick := ctx.Args[0]

	// Send MODE -v command to IRC server
	err := c.client.Write(&irc.Message{
		Command: "MODE",
		Params:  []string{ctx.Channel, "-v", nick},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to devoice user: %w", err)
	}

	return NewResponse(fmt.Sprintf("Removed voice from %s in %s", nick, ctx.Channel)), nil
}

// RequiredPermission returns the minimum permission level needed
// Requirement 19.2: Admin or owner can use channel management commands
func (c *DevoiceCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for this command
func (c *DevoiceCommand) Help() string {
	return "!devoice <nick> - Remove voice from a user (admin/owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *DevoiceCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for devoice command
}

// TopicCommand implements the !topic command
// Requirement 19.2: Support topic operation for admin/owner
type TopicCommand struct {
	client IRCClient
}

// NewTopicCommand creates a new topic command
func NewTopicCommand(client IRCClient) *TopicCommand {
	return &TopicCommand{
		client: client,
	}
}

// Name returns the command name
func (c *TopicCommand) Name() string {
	return "topic"
}

// Execute runs the topic command
// Requirement 19.8: Channel operator status verification
// Note: The IRC server will reject the command if the bot lacks operator status.
func (c *TopicCommand) Execute(ctx *Context) (*Response, error) {
	// Must be used in a channel, not PM
	if ctx.IsPM {
		return NewErrorResponse("This command can only be used in a channel."), nil
	}

	// Check arguments: !topic <new topic>
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !topic <new topic>"), nil
	}

	newTopic := strings.Join(ctx.Args, " ")

	// Send TOPIC command to IRC server
	err := c.client.Write(&irc.Message{
		Command: "TOPIC",
		Params:  []string{ctx.Channel, newTopic},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to set topic: %w", err)
	}

	return NewResponse(fmt.Sprintf("Set topic in %s", ctx.Channel)), nil
}

// RequiredPermission returns the minimum permission level needed
// Requirement 19.2: Admin or owner can use channel management commands
func (c *TopicCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for this command
func (c *TopicCommand) Help() string {
	return "!topic <new topic> - Set the channel topic (admin/owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *TopicCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for topic command
}

// TopicAppendCommand implements the !topicappend command
// Requirement 19.2: Support topicappend operation for admin/owner
type TopicAppendCommand struct {
	client IRCClient
}

// NewTopicAppendCommand creates a new topicappend command
func NewTopicAppendCommand(client IRCClient) *TopicAppendCommand {
	return &TopicAppendCommand{
		client: client,
	}
}

// Name returns the command name
func (c *TopicAppendCommand) Name() string {
	return "topicappend"
}

// Execute runs the topicappend command
// Requirement 19.8: Channel operator status verification
// Note: The IRC server will reject the command if the bot lacks operator status.
// Note: This command requires the bot to first retrieve the current topic,
// which is not implemented in this simplified version. In a full implementation,
// the bot would need to track channel topics or query them before appending.
func (c *TopicAppendCommand) Execute(ctx *Context) (*Response, error) {
	// Must be used in a channel, not PM
	if ctx.IsPM {
		return NewErrorResponse("This command can only be used in a channel."), nil
	}

	// Check arguments: !topicappend <text to append>
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !topicappend <text to append>"), nil
	}

	// Note: In a full implementation, we would need to:
	// 1. Request the current topic with TOPIC command
	// 2. Wait for the 332 (RPL_TOPIC) response
	// 3. Append the new text
	// 4. Set the new topic
	//
	// For this POC, we'll return an informative message
	return NewErrorResponse("Topic append requires tracking current topic. Use !topic to set a new topic instead."), nil
}

// RequiredPermission returns the minimum permission level needed
// Requirement 19.2: Admin or owner can use channel management commands
func (c *TopicAppendCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for this command
func (c *TopicAppendCommand) Help() string {
	return "!topicappend <text> - Append text to the channel topic (admin/owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *TopicAppendCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for topicappend command
}

// ModeCommand implements the !mode command
// Requirement 19.2: Support mode operation for admin/owner
type ModeCommand struct {
	client IRCClient
}

// NewModeCommand creates a new mode command
func NewModeCommand(client IRCClient) *ModeCommand {
	return &ModeCommand{
		client: client,
	}
}

// Name returns the command name
func (c *ModeCommand) Name() string {
	return "mode"
}

// Execute runs the mode command
// Requirement 19.8: Channel operator status verification
// Note: The IRC server will reject the command if the bot lacks operator status.
func (c *ModeCommand) Execute(ctx *Context) (*Response, error) {
	// Must be used in a channel, not PM
	if ctx.IsPM {
		return NewErrorResponse("This command can only be used in a channel."), nil
	}

	// Check arguments: !mode <mode string> [args...]
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !mode <mode string> [args...]"), nil
	}

	// Build MODE command params: channel, mode string, and any additional args
	params := []string{ctx.Channel}
	params = append(params, ctx.Args...)

	// Send MODE command to IRC server
	err := c.client.Write(&irc.Message{
		Command: "MODE",
		Params:  params,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to set mode: %w", err)
	}

	return NewResponse(fmt.Sprintf("Set mode %s in %s", ctx.Args[0], ctx.Channel)), nil
}

// RequiredPermission returns the minimum permission level needed
// Requirement 19.2: Admin or owner can use channel management commands
func (c *ModeCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for this command
func (c *ModeCommand) Help() string {
	return "!mode <mode string> [args...] - Set channel modes (admin/owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *ModeCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for mode command
}

// InviteCommand implements the !invite command
// Requirement 19.2: Support invite operation for admin/owner
type InviteCommand struct {
	client IRCClient
}

// NewInviteCommand creates a new invite command
func NewInviteCommand(client IRCClient) *InviteCommand {
	return &InviteCommand{
		client: client,
	}
}

// Name returns the command name
func (c *InviteCommand) Name() string {
	return "invite"
}

// Execute runs the invite command
// Requirement 19.8: Channel operator status verification
// Note: The IRC server will reject the command if the bot lacks operator status.
func (c *InviteCommand) Execute(ctx *Context) (*Response, error) {
	// Must be used in a channel, not PM
	if ctx.IsPM {
		return NewErrorResponse("This command can only be used in a channel."), nil
	}

	// Check arguments: !invite <nick>
	if len(ctx.Args) < 1 {
		return NewErrorResponse("Usage: !invite <nick>"), nil
	}

	nick := ctx.Args[0]

	// Send INVITE command to IRC server
	err := c.client.Write(&irc.Message{
		Command: "INVITE",
		Params:  []string{nick, ctx.Channel},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to invite user: %w", err)
	}

	return NewResponse(fmt.Sprintf("Invited %s to %s", nick, ctx.Channel)), nil
}

// RequiredPermission returns the minimum permission level needed
// Requirement 19.2: Admin or owner can use channel management commands
func (c *InviteCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for this command
func (c *InviteCommand) Help() string {
	return "!invite <nick> - Invite a user to the channel (admin/owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *InviteCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for invite command
}
