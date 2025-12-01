package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/errors"
	"github.com/yourusername/lolo/internal/user"
)

// UserAddCommand implements the !user add command
type UserAddCommand struct {
	userManager *user.Manager
	db          *database.DB
}

// NewUserAddCommand creates a new user add command
func NewUserAddCommand(userManager *user.Manager, db *database.DB) *UserAddCommand {
	return &UserAddCommand{
		userManager: userManager,
		db:          db,
	}
}

// Name returns the command name
func (c *UserAddCommand) Name() string {
	return "user add"
}

// Execute runs the user add command
func (c *UserAddCommand) Execute(ctx *Context) (*Response, error) {
	// Check arguments: !user add <nick> <level>
	if len(ctx.Args) < 2 {
		return nil, errors.NewInvalidSyntaxError("user add", "!user add <nick> <level> (levels: ignored, normal, admin)")
	}

	nick := ctx.Args[0]
	levelStr := strings.ToLower(ctx.Args[1])

	// Parse permission level
	var level database.PermissionLevel
	switch levelStr {
	case "ignored":
		level = database.LevelIgnored
	case "normal":
		level = database.LevelNormal
	case "admin":
		level = database.LevelAdmin
	case "owner":
		// Owner level cannot be set via !user add (Requirement 15.5)
		return nil, errors.NewValidationError("Cannot add users with owner level. Owner is set via !verify command.")
	default:
		return nil, errors.NewValidationError(fmt.Sprintf("Invalid permission level: %s. Valid levels: ignored, normal, admin", levelStr))
	}

	// Admin restriction: admins can only add normal users (Requirement 15.5)
	if ctx.UserLevel == database.LevelAdmin && level != database.LevelNormal {
		return nil, errors.NewPermissionError(database.LevelOwner)
	}

	// Check if user already exists
	existingUser, err := c.userManager.GetUser(nick)
	if err != nil {
		return nil, errors.NewDatabaseError("check existing user", err)
	}
	if existingUser != nil {
		return nil, errors.NewValidationError(fmt.Sprintf("User %s already exists with level %s.", nick, errors.PermissionLevelName(existingUser.Level)))
	}

	// Add the user with a placeholder hostmask (will be updated on first WHOIS)
	// We use "*" as placeholder since we don't have the hostmask yet
	err = c.userManager.AddUser(nick, "*", level)
	if err != nil {
		return nil, errors.NewDatabaseError("add user", err)
	}

	// Log audit action (Requirement 15.7, 29.1)
	auditErr := c.db.LogAuditAction(
		ctx.Nick,
		ctx.Hostmask,
		"user_add",
		nick,
		fmt.Sprintf("level=%s", errors.PermissionLevelName(level)),
		"success",
	)
	if auditErr != nil {
		// Log the error but don't fail the command
		fmt.Printf("Warning: failed to log audit action: %v\n", auditErr)
	}

	return NewResponse(fmt.Sprintf("User %s added with %s level.", nick, errors.PermissionLevelName(level))), nil
}

// RequiredPermission returns the minimum permission level needed
// Both owner and admin can add users (Requirement 15.1, 15.4)
func (c *UserAddCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for this command
func (c *UserAddCommand) Help() string {
	return "!user add <nick> <level> - Add a user with specified permission level (owner/admin only; admins can only add normal users)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *UserAddCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for user add command
}

// UserRemoveCommand implements the !user remove command
type UserRemoveCommand struct {
	userManager *user.Manager
	db          *database.DB
}

// NewUserRemoveCommand creates a new user remove command
func NewUserRemoveCommand(userManager *user.Manager, db *database.DB) *UserRemoveCommand {
	return &UserRemoveCommand{
		userManager: userManager,
		db:          db,
	}
}

// Name returns the command name
func (c *UserRemoveCommand) Name() string {
	return "user remove"
}

// Execute runs the user remove command
func (c *UserRemoveCommand) Execute(ctx *Context) (*Response, error) {
	// Check arguments: !user remove <nick>
	if len(ctx.Args) < 1 {
		return nil, errors.NewInvalidSyntaxError("user remove", "!user remove <nick>")
	}

	nick := ctx.Args[0]

	// Check if user exists
	existingUser, err := c.userManager.GetUser(nick)
	if err != nil {
		return nil, errors.NewDatabaseError("check user", err)
	}
	if existingUser == nil {
		return nil, errors.NewNotFoundError("User", nick)
	}

	// Remove the user (RemoveUser already prevents removing owner)
	err = c.userManager.RemoveUser(nick)
	if err != nil {
		// Check if it's the "cannot remove owner" error
		if strings.Contains(err.Error(), "cannot remove the owner") {
			return nil, errors.NewValidationError("Cannot remove the owner.")
		}
		return nil, errors.NewDatabaseError("remove user", err)
	}

	// Log audit action (Requirement 15.7, 29.1)
	auditErr := c.db.LogAuditAction(
		ctx.Nick,
		ctx.Hostmask,
		"user_remove",
		nick,
		"",
		"success",
	)
	if auditErr != nil {
		// Log the error but don't fail the command
		fmt.Printf("Warning: failed to log audit action: %v\n", auditErr)
	}

	return NewResponse(fmt.Sprintf("User %s removed.", nick)), nil
}

// RequiredPermission returns the minimum permission level needed
// Only owner can remove users (Requirement 15.2)
func (c *UserRemoveCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelOwner
}

// Help returns help text for this command
func (c *UserRemoveCommand) Help() string {
	return "!user remove <nick> - Remove a user (owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *UserRemoveCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for user remove command
}

// UserListCommand implements the !user list command
type UserListCommand struct {
	userManager *user.Manager
}

// NewUserListCommand creates a new user list command
func NewUserListCommand(userManager *user.Manager) *UserListCommand {
	return &UserListCommand{
		userManager: userManager,
	}
}

// Name returns the command name
func (c *UserListCommand) Name() string {
	return "user list"
}

// Execute runs the user list command
func (c *UserListCommand) Execute(ctx *Context) (*Response, error) {
	// Get all users
	users, err := c.userManager.ListUsers()
	if err != nil {
		return nil, errors.NewDatabaseError("list users", err)
	}

	if len(users) == 0 {
		return NewPMResponse("No users registered."), nil
	}

	// Build the user list message
	var sb strings.Builder
	sb.WriteString("Registered users:\n")
	for _, u := range users {
		sb.WriteString(fmt.Sprintf("  %s - %s (hostmask: %s)\n", u.Nick, errors.PermissionLevelName(u.Level), u.Hostmask))
	}

	// Send as PM (Requirement 15.3)
	return NewPMResponse(sb.String()), nil
}

// RequiredPermission returns the minimum permission level needed
// Both owner and admin can list users (Requirement 15.3)
func (c *UserListCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

// Help returns help text for this command
func (c *UserListCommand) Help() string {
	return "!user list - List all registered users (owner/admin only, sent via PM)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *UserListCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for user list command
}
