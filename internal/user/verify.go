package user

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/yourusername/lolo/internal/database"
)

const (
	// OwnerPasswordKey is the key used to store the owner password hash in bot_settings
	OwnerPasswordKey = "owner_password_hash"
	// BcryptCost is the cost factor for bcrypt hashing
	BcryptCost = 12
)

// SetOwnerPassword sets the owner verification password (hashed with bcrypt)
func (m *Manager) SetOwnerPassword(password string) error {
	// Hash the password using bcrypt
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Store the hash in bot_settings
	if err := m.db.SetSetting(OwnerPasswordKey, string(hash)); err != nil {
		return fmt.Errorf("failed to store password hash: %w", err)
	}

	return nil
}

// GetOwnerPasswordHash retrieves the owner password hash
func (m *Manager) GetOwnerPasswordHash() (string, error) {
	hash, err := m.db.GetSetting(OwnerPasswordKey)
	if err != nil {
		return "", fmt.Errorf("failed to get password hash: %w", err)
	}
	return hash, nil
}

// VerifyOwnerPassword verifies a password against the stored hash
func (m *Manager) VerifyOwnerPassword(password string) (bool, error) {
	// Get the stored hash
	hash, err := m.GetOwnerPasswordHash()
	if err != nil {
		return false, err
	}

	if hash == "" {
		return false, fmt.Errorf("no owner password set")
	}

	// Compare the password with the hash
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err == bcrypt.ErrMismatchedHashAndPassword {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to verify password: %w", err)
	}

	return true, nil
}

// SetOwner sets a user as the owner after password verification
func (m *Manager) SetOwner(nick, hostmask, password string) error {
	// Check if owner already exists
	hasOwner, err := m.HasOwner()
	if err != nil {
		return fmt.Errorf("failed to check for owner: %w", err)
	}
	if hasOwner {
		return fmt.Errorf("owner already exists")
	}

	// Verify the password
	valid, err := m.VerifyOwnerPassword(password)
	if err != nil {
		return fmt.Errorf("failed to verify password: %w", err)
	}
	if !valid {
		return fmt.Errorf("invalid password")
	}

	// Check if user already exists
	user, err := m.db.GetUser(nick)
	if err != nil {
		return fmt.Errorf("failed to check existing user: %w", err)
	}

	if user != nil {
		// Update existing user to owner
		user.Level = database.LevelOwner
		user.Hostmask = hostmask
		if err := m.db.UpdateUser(user); err != nil {
			return fmt.Errorf("failed to update user to owner: %w", err)
		}
	} else {
		// Create new owner user
		user = &database.User{
			Nick:     nick,
			Hostmask: hostmask,
			Level:    database.LevelOwner,
		}
		if err := m.db.CreateUser(user); err != nil {
			return fmt.Errorf("failed to create owner: %w", err)
		}
	}

	return nil
}

// HasOwnerPassword checks if an owner password has been set
func (m *Manager) HasOwnerPassword() (bool, error) {
	hash, err := m.GetOwnerPasswordHash()
	if err != nil {
		return false, err
	}
	return hash != "", nil
}
