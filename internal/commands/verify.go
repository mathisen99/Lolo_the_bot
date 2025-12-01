package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/user"
)

// VerifyCommand implements the !verify command for owner verification
type VerifyCommand struct {
	userManager *user.Manager
	db          *database.DB
}

// NewVerifyCommand creates a new verify command
func NewVerifyCommand(userManager *user.Manager, db *database.DB) *VerifyCommand {
	return &VerifyCommand{
		userManager: userManager,
		db:          db,
	}
}

// Name returns the command name
func (c *VerifyCommand) Name() string {
	return "verify"
}

// Execute runs the verify command
func (c *VerifyCommand) Execute(ctx *Context) (*Response, error) {
	// CRITICAL: Only allow !verify via PM, never in channels (Requirement 11.6)
	if !ctx.IsPM {
		// Log channel !verify attempts as security events (Requirement 11.6)
		// This is a security feature - we log the attempt but don't acknowledge it
		_ = c.db.LogAuditAction(
			ctx.Nick,
			ctx.Hostmask,
			"verify_channel_attempt",
			"",
			fmt.Sprintf("Attempted !verify in channel: %s", ctx.Channel),
			"blocked",
		)
		// Silently ignore channel !verify attempts to prevent password exposure
		return nil, nil
	}

	// Check if password was provided
	if len(ctx.Args) == 0 {
		return NewErrorResponse("Usage: !verify <password>"), nil
	}

	// Get the password from arguments
	password := strings.Join(ctx.Args, " ")

	// Check if owner already exists (Requirement 11.5)
	hasOwner, err := c.userManager.HasOwner()
	if err != nil {
		return nil, fmt.Errorf("failed to check for owner: %w", err)
	}

	if hasOwner {
		// Owner already exists - reject the verification attempt
		return NewPMResponse("Owner already exists. Verification is no longer available."), nil
	}

	// Verify the password and set the user as owner (Requirement 11.4)
	err = c.userManager.SetOwner(ctx.Nick, ctx.Hostmask, password)
	if err != nil {
		// Check if it's an invalid password error
		if strings.Contains(err.Error(), "invalid password") {
			return NewPMResponse("Invalid password. Please try again."), nil
		}
		// Other errors (database issues, etc.)
		return nil, fmt.Errorf("failed to set owner: %w", err)
	}

	// Success! User is now the owner
	return NewPMResponse("Owner verified! You are now the bot owner."), nil
}

// RequiredPermission returns the minimum permission level needed
// Note: This command has special logic - it doesn't require any permission level
// because it's used to establish the first owner. However, we return LevelNormal
// to allow anyone to attempt it, and the command itself enforces the business logic.
func (c *VerifyCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelNormal
}

// Help returns help text for this command
func (c *VerifyCommand) Help() string {
	return "!verify <password> - Verify yourself as the bot owner (PM only, only works when no owner exists)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *VerifyCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for verify command
}
