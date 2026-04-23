package commands

import (
	"fmt"
	"strings"
	"sync"

	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/user"
)

// Dispatcher handles command detection and routing
type Dispatcher struct {
	registry        *Registry
	userManager     *user.Manager
	defaultPrefix   string
	channelPrefixes map[string]string
	mu              sync.RWMutex
}

// NewDispatcher creates a new command dispatcher
func NewDispatcher(registry *Registry, userManager *user.Manager, commandPrefix string) *Dispatcher {
	return &Dispatcher{
		registry:        registry,
		userManager:     userManager,
		defaultPrefix:   commandPrefix,
		channelPrefixes: make(map[string]string),
	}
}

// IsCommand checks if a message is a command in the given context.
func (d *Dispatcher) IsCommand(channel, message string, isPM bool) bool {
	return strings.HasPrefix(message, d.GetActivePrefix(channel, isPM))
}

// ParseCommand parses a message into command name and arguments
// Returns empty string if the message is not a command
// Supports multi-word commands (e.g., "user add", "channel enable")
func (d *Dispatcher) ParseCommand(channel, message string, isPM bool) (command string, args []string) {
	prefix := d.GetActivePrefix(channel, isPM)
	if !strings.HasPrefix(message, prefix) {
		return "", nil
	}

	// Remove the prefix
	message = strings.TrimPrefix(message, prefix)
	message = strings.TrimSpace(message)

	// Split into command and arguments
	parts := strings.Fields(message)
	if len(parts) == 0 {
		return "", nil
	}

	// Try to match multi-word commands first (e.g., "user add", "channel enable")
	// Check for 2-word commands
	if len(parts) >= 2 {
		twoWordCmd := strings.ToLower(parts[0] + " " + parts[1])
		if d.registry.Has(twoWordCmd) {
			command = twoWordCmd
			if len(parts) > 2 {
				args = parts[2:]
			} else {
				args = []string{}
			}
			return command, args
		}
	}

	// Fall back to single-word command
	command = strings.ToLower(parts[0])
	if len(parts) > 1 {
		args = parts[1:]
	} else {
		// Return empty slice instead of nil to avoid JSON null serialization
		args = []string{}
	}

	return command, args
}

// Dispatch processes a message and executes the command if applicable
// Returns the response, whether it was a command, and any error
func (d *Dispatcher) Dispatch(nick, hostmask, channel, message string, isPM bool) (*Response, bool, error) {
	// Check if this is a command
	if !d.IsCommand(channel, message, isPM) {
		return nil, false, nil
	}

	// Parse the command
	command, args := d.ParseCommand(channel, message, isPM)
	if command == "" {
		return nil, false, nil
	}

	// Get user information and permission level
	userLevel, isRegistered, err := d.getUserPermissionLevel(nick, hostmask)
	if err != nil {
		return nil, true, fmt.Errorf("failed to get user permission level: %w", err)
	}

	// Create command context
	ctx := NewContext(command, args, message, nick, hostmask, channel, isPM, userLevel, isRegistered, d.GetActivePrefix(channel, isPM))

	// Check command cooldown before executing (Requirement 15.8, 15.9)
	// Get the command to check its cooldown duration
	cmd, exists := d.registry.Get(command)
	if exists {
		cooldownDuration := cmd.CooldownDuration()
		if cooldownDuration > 0 {
			remaining := d.userManager.CheckCommandCooldown(nick, command, cooldownDuration)
			if remaining > 0 {
				// User is on cooldown
				return &Response{
					Message: fmt.Sprintf("Command on cooldown. Please wait %.1f seconds.", remaining.Seconds()),
				}, true, nil
			}
		}
	}

	// Execute the command
	response, err := d.registry.Execute(ctx)
	if err != nil {
		return nil, true, err
	}

	// Record the command usage for cooldown tracking (Requirement 15.8, 15.9)
	if exists {
		cooldownDuration := cmd.CooldownDuration()
		if cooldownDuration > 0 {
			if err := d.userManager.SetCommandCooldown(nick, command); err != nil {
				// Log the error but don't fail the command execution
				fmt.Printf("Warning: failed to set command cooldown: %v\n", err)
			}
		}
	}

	return response, true, nil
}

// getUserPermissionLevel retrieves the user's permission level
// Returns the permission level, whether the user is registered, and any error
func (d *Dispatcher) getUserPermissionLevel(nick, hostmask string) (database.PermissionLevel, bool, error) {
	// Try to get user by nickname first
	user, err := d.userManager.GetUser(nick)
	if err != nil {
		return database.LevelNormal, false, fmt.Errorf("failed to get user by nick: %w", err)
	}

	// If found by nick, return their level
	if user != nil {
		return user.Level, true, nil
	}

	// If not found by nick and we have a hostmask, try by hostmask
	if hostmask != "" {
		user, err = d.userManager.GetUserByHostmask(hostmask)
		if err != nil {
			return database.LevelNormal, false, fmt.Errorf("failed to get user by hostmask: %w", err)
		}

		if user != nil {
			return user.Level, true, nil
		}
	}

	// User not found - they are unregistered with normal level (no special permissions)
	return database.LevelNormal, false, nil
}

// GetRegistry returns the command registry
func (d *Dispatcher) GetRegistry() *Registry {
	return d.registry
}

// GetActivePrefix returns the effective command prefix for the context.
func (d *Dispatcher) GetActivePrefix(channel string, isPM bool) string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if isPM || channel == "" {
		return d.defaultPrefix
	}
	if prefix, exists := d.channelPrefixes[channel]; exists && prefix != "" {
		return prefix
	}
	return d.defaultPrefix
}

// GetDefaultPrefix returns the configured default command prefix.
func (d *Dispatcher) GetDefaultPrefix() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.defaultPrefix
}

// SetChannelPrefix updates the command prefix override for a channel.
func (d *Dispatcher) SetChannelPrefix(channel, prefix string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.channelPrefixes[channel] = prefix
}

// ClearChannelPrefix removes the command prefix override for a channel.
func (d *Dispatcher) ClearChannelPrefix(channel string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.channelPrefixes, channel)
}
