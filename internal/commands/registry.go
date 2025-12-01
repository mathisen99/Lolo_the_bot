package commands

import (
	"fmt"
	"strings"
	"sync"

	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/errors"
)

// Registry manages command registration and dispatch
type Registry struct {
	commands map[string]Command
	mu       sync.RWMutex
}

// NewRegistry creates a new command registry
func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]Command),
	}
}

// Register adds a command to the registry
func (r *Registry) Register(cmd Command) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := strings.ToLower(cmd.Name())
	if _, exists := r.commands[name]; exists {
		return fmt.Errorf("command %s already registered", name)
	}

	r.commands[name] = cmd
	return nil
}

// Get retrieves a command by name
func (r *Registry) Get(name string) (Command, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cmd, exists := r.commands[strings.ToLower(name)]
	return cmd, exists
}

// Has checks if a command exists in the registry
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.commands[strings.ToLower(name)]
	return exists
}

// List returns all registered command names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.commands))
	for name := range r.commands {
		names = append(names, name)
	}
	return names
}

// GetAll returns all registered commands
func (r *Registry) GetAll() []Command {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cmds := make([]Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		cmds = append(cmds, cmd)
	}
	return cmds
}

// Execute executes a command with the given context
// Returns an error if the command doesn't exist or permission is denied
func (r *Registry) Execute(ctx *Context) (*Response, error) {
	cmd, exists := r.Get(ctx.Command)
	if !exists {
		return nil, errors.NewNotFoundError("Command", ctx.Command)
	}

	// Check permissions
	required := cmd.RequiredPermission()
	if !HasPermission(ctx.UserLevel, required) {
		return nil, errors.NewPermissionError(required)
	}

	// Execute the command
	return cmd.Execute(ctx)
}

// HasPermission checks if a user's permission level meets the required level
func HasPermission(userLevel, required database.PermissionLevel) bool {
	// Ignored users have no permissions
	if userLevel == database.LevelIgnored {
		return false
	}

	// Check if user level meets or exceeds required level
	return userLevel >= required
}

// PermissionLevelName returns a human-readable name for a permission level
func PermissionLevelName(level database.PermissionLevel) string {
	switch level {
	case database.LevelIgnored:
		return "ignored"
	case database.LevelNormal:
		return "normal"
	case database.LevelAdmin:
		return "admin"
	case database.LevelOwner:
		return "owner"
	default:
		return "unknown"
	}
}
