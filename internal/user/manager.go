package user

import (
	"fmt"
	"time"

	"github.com/yourusername/lolo/internal/database"
)

// Manager handles user management and permission checking
type Manager struct {
	db         *database.DB
	whoisCache *WhoisCache
}

// NewManager creates a new user manager
func NewManager(db *database.DB) *Manager {
	return &Manager{
		db:         db,
		whoisCache: NewWhoisCache(1 * time.Hour), // 1 hour TTL as per requirements
	}
}

// GetUser retrieves a user by nickname
func (m *Manager) GetUser(nick string) (*database.User, error) {
	return m.db.GetUser(nick)
}

// GetUserByHostmask retrieves a user by hostmask
func (m *Manager) GetUserByHostmask(hostmask string) (*database.User, error) {
	return m.db.GetUserByHostmask(hostmask)
}

// AddUser adds a new user with the specified permission level
func (m *Manager) AddUser(nick, hostmask string, level database.PermissionLevel) error {
	// Check if user already exists
	existing, err := m.db.GetUser(nick)
	if err != nil {
		return fmt.Errorf("failed to check existing user: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("user %s already exists", nick)
	}

	// Create the user
	user := &database.User{
		Nick:     nick,
		Hostmask: hostmask,
		Level:    level,
	}

	if err := m.db.CreateUser(user); err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// RemoveUser removes a user by nickname
func (m *Manager) RemoveUser(nick string) error {
	// Check if user exists
	user, err := m.db.GetUser(nick)
	if err != nil {
		return fmt.Errorf("failed to check user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user %s not found", nick)
	}

	// Don't allow removing the owner
	if user.Level == database.LevelOwner {
		return fmt.Errorf("cannot remove the owner")
	}

	if err := m.db.DeleteUser(nick); err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	return nil
}

// ListUsers returns all users
func (m *Manager) ListUsers() ([]*database.User, error) {
	return m.db.ListUsers()
}

// CheckPermission checks if a user has the required permission level
// Returns true if the user has sufficient permissions, false otherwise
func (m *Manager) CheckPermission(nick, hostmask string, required database.PermissionLevel) (bool, error) {
	// First try to get user by nickname
	user, err := m.db.GetUser(nick)
	if err != nil {
		return false, fmt.Errorf("failed to get user by nick: %w", err)
	}

	// If not found by nick, try by hostmask
	if user == nil && hostmask != "" {
		user, err = m.db.GetUserByHostmask(hostmask)
		if err != nil {
			return false, fmt.Errorf("failed to get user by hostmask: %w", err)
		}
	}

	// If user not found, they have no permissions
	if user == nil {
		return false, nil
	}

	// Ignored users have no permissions
	if user.Level == database.LevelIgnored {
		return false, nil
	}

	// Check if user's level meets or exceeds required level
	return user.Level >= required, nil
}

// UpdateHostmask updates a user's hostmask
func (m *Manager) UpdateHostmask(nick, hostmask string) error {
	user, err := m.db.GetUser(nick)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user %s not found", nick)
	}

	user.Hostmask = hostmask
	if err := m.db.UpdateUser(user); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// HasOwner checks if an owner exists in the database
func (m *Manager) HasOwner() (bool, error) {
	return m.db.HasOwner()
}

// GetOwner retrieves the owner user
func (m *Manager) GetOwner() (*database.User, error) {
	users, err := m.db.ListUsers()
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	for _, user := range users {
		if user.Level == database.LevelOwner {
			return user, nil
		}
	}

	return nil, nil
}

// GetCachedHostmask retrieves a hostmask from the WHOIS cache
// Returns empty string if not cached or expired
func (m *Manager) GetCachedHostmask(nick string) string {
	entry := m.whoisCache.Get(nick)
	if entry == nil {
		return ""
	}
	return entry.Hostmask
}

// SetCachedHostmask stores a hostmask in the WHOIS cache
func (m *Manager) SetCachedHostmask(nick, hostmask string) {
	m.whoisCache.Set(nick, hostmask)
}

// IsCacheExpired checks if a WHOIS cache entry has expired
func (m *Manager) IsCacheExpired(nick string) bool {
	return m.whoisCache.IsExpired(nick)
}

// InvalidateWhoisCache removes a user from the WHOIS cache
func (m *Manager) InvalidateWhoisCache(nick string) {
	m.whoisCache.Delete(nick)
}

// InvalidateMultipleWhoisCache removes multiple users from the WHOIS cache
// This is useful for handling netsplits
func (m *Manager) InvalidateMultipleWhoisCache(nicks []string) {
	m.whoisCache.InvalidateMultiple(nicks)
}

// ClearWhoisCache removes all entries from the WHOIS cache
func (m *Manager) ClearWhoisCache() {
	m.whoisCache.Clear()
}

// GetWhoisCacheSize returns the number of entries in the WHOIS cache
func (m *Manager) GetWhoisCacheSize() int {
	return m.whoisCache.Size()
}

// CleanupExpiredWhoisEntries removes expired entries from the WHOIS cache
// Returns the number of entries removed
func (m *Manager) CleanupExpiredWhoisEntries() int {
	return m.whoisCache.CleanupExpired()
}

// ShouldRequestWhois determines if a WHOIS request should be sent for a user
// Returns true if the user is not in cache or the cache entry has expired
func (m *Manager) ShouldRequestWhois(nick string) bool {
	entry := m.whoisCache.Get(nick)
	return entry == nil
}

// UpdateHostmaskFromWhois updates both the database and cache with a new hostmask
// This should be called when a WHOIS response is received
func (m *Manager) UpdateHostmaskFromWhois(nick, hostmask string) error {
	// Update cache first (always succeeds)
	m.SetCachedHostmask(nick, hostmask)

	// Try to update database if user exists
	user, err := m.db.GetUser(nick)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// If user exists in database, update their hostmask
	if user != nil {
		user.Hostmask = hostmask
		if err := m.db.UpdateUser(user); err != nil {
			return fmt.Errorf("failed to update user hostmask: %w", err)
		}
	}

	return nil
}

// IsRegisteredUser checks if a user exists in the database
// This is used to determine if WHOIS should be performed for a user
// CRITICAL: Only registered users should be WHOIS'd to avoid flooding IRC servers
func (m *Manager) IsRegisteredUser(nick string) (bool, error) {
	user, err := m.db.GetUser(nick)
	if err != nil {
		return false, fmt.Errorf("failed to check user: %w", err)
	}
	return user != nil, nil
}

// CheckCommandCooldown checks if a user is on cooldown for a command
// Returns the remaining cooldown duration if on cooldown, or 0 if not on cooldown
// Requirement 15.8, 15.9
func (m *Manager) CheckCommandCooldown(nick, commandName string, cooldownDuration time.Duration) time.Duration {
	return m.db.CheckCommandCooldown(nick, commandName, cooldownDuration)
}

// SetCommandCooldown records the last time a user used a command
// Requirement 15.8, 15.9
func (m *Manager) SetCommandCooldown(nick, commandName string) error {
	return m.db.SetCommandCooldown(nick, commandName)
}

// ClearCommandCooldown removes a cooldown record for a user and command
func (m *Manager) ClearCommandCooldown(nick, commandName string) error {
	return m.db.ClearCommandCooldown(nick, commandName)
}

// ClearUserCooldowns removes all cooldown records for a user
func (m *Manager) ClearUserCooldowns(nick string) error {
	return m.db.ClearUserCooldowns(nick)
}
