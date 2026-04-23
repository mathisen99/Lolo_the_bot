package commands

import (
	"fmt"
	"time"

	"github.com/yourusername/lolo/internal/database"
	boterrors "github.com/yourusername/lolo/internal/errors"
)

// PrefixCommand manages per-channel command prefix overrides.
type PrefixCommand struct {
	db         *database.DB
	dispatcher *Dispatcher
}

// NewPrefixCommand creates a new prefix command.
func NewPrefixCommand(db *database.DB, dispatcher *Dispatcher) *PrefixCommand {
	return &PrefixCommand{
		db:         db,
		dispatcher: dispatcher,
	}
}

// Name returns the command name.
func (c *PrefixCommand) Name() string {
	return "prefix"
}

// Execute runs the prefix command.
func (c *PrefixCommand) Execute(ctx *Context) (*Response, error) {
	if ctx.IsPM {
		return NewErrorResponse("Prefix is channel-specific. Use this command in a channel."), nil
	}

	if len(ctx.Args) != 1 {
		return nil, boterrors.NewInvalidSyntaxError("prefix", ctx.ActivePrefix+"prefix show | "+ctx.ActivePrefix+"prefix <symbol>")
	}

	arg := ctx.Args[0]
	defaultPrefix := c.dispatcher.GetDefaultPrefix()

	if arg == "show" {
		activePrefix := c.dispatcher.GetActivePrefix(ctx.Channel, ctx.IsPM)
		if activePrefix == defaultPrefix {
			return NewResponse(fmt.Sprintf("Active command prefix for %s is %q (default). Use %sprefix <symbol> to set a custom prefix.", ctx.Channel, activePrefix, ctx.ActivePrefix)), nil
		}
		return NewResponse(fmt.Sprintf("Active command prefix for %s is %q (custom). Use %sprefix %s to reset it.", ctx.Channel, activePrefix, activePrefix, defaultPrefix)), nil
	}

	if !isValidCommandPrefix(arg) {
		return NewErrorResponse("Prefix must be a single symbol character, for example !, -, ., or ?"), nil
	}

	if arg == defaultPrefix {
		if err := c.db.ClearChannelCommandPrefix(ctx.Channel); err != nil {
			return nil, fmt.Errorf("failed to clear channel command prefix: %w", err)
		}
		c.dispatcher.ClearChannelPrefix(ctx.Channel)
		c.logAudit(ctx, "channel_prefix_reset", fmt.Sprintf("channel=%s", ctx.Channel))
		return NewResponse(fmt.Sprintf("Command prefix for %s reset to %q. Use %sprefix <symbol> to set a custom prefix again.", ctx.Channel, defaultPrefix, defaultPrefix)), nil
	}

	if err := c.db.SetChannelCommandPrefix(ctx.Channel, arg); err != nil {
		return nil, fmt.Errorf("failed to set channel command prefix: %w", err)
	}
	c.dispatcher.SetChannelPrefix(ctx.Channel, arg)
	c.logAudit(ctx, "channel_prefix_set", fmt.Sprintf("channel=%s prefix=%s", ctx.Channel, arg))

	return NewResponse(fmt.Sprintf("Command prefix for %s set to %q. Use %sprefix %s to reset it.", ctx.Channel, arg, arg, defaultPrefix)), nil
}

// RequiredPermission returns the minimum permission level needed.
func (c *PrefixCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for the command.
func (c *PrefixCommand) Help() string {
	return "!prefix show | !prefix <symbol> - Show or change the command prefix for this channel (admin/owner only)"
}

// CooldownDuration returns the cooldown duration for this command.
func (c *PrefixCommand) CooldownDuration() time.Duration {
	return 0
}

func (c *PrefixCommand) logAudit(ctx *Context, action, details string) {
	if c.db == nil {
		return
	}
	if err := c.db.LogAuditAction(ctx.Nick, ctx.Hostmask, action, "", details, "success"); err != nil {
		fmt.Printf("Warning: failed to log audit action: %v\n", err)
	}
}

func isValidCommandPrefix(prefix string) bool {
	if len(prefix) != 1 {
		return false
	}

	ch := prefix[0]
	if ch < 33 || ch > 126 {
		return false
	}
	if (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
		return false
	}

	return true
}
