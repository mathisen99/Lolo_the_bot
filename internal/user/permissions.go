package user

import (
	"fmt"

	"github.com/yourusername/lolo/internal/database"
)

// PermissionLevelToString converts a permission level to a string
func PermissionLevelToString(level database.PermissionLevel) string {
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

// StringToPermissionLevel converts a string to a permission level
func StringToPermissionLevel(s string) (database.PermissionLevel, error) {
	switch s {
	case "ignored":
		return database.LevelIgnored, nil
	case "normal":
		return database.LevelNormal, nil
	case "admin":
		return database.LevelAdmin, nil
	case "owner":
		return database.LevelOwner, nil
	default:
		return 0, fmt.Errorf("invalid permission level: %s", s)
	}
}

// CanManageUser checks if an actor can manage a target user
// Owners can manage anyone
// Admins can only manage normal users
// Normal users cannot manage anyone
func CanManageUser(actorLevel, targetLevel database.PermissionLevel) bool {
	// Owner can manage anyone
	if actorLevel == database.LevelOwner {
		return true
	}

	// Admin can only manage normal and ignored users
	if actorLevel == database.LevelAdmin {
		return targetLevel == database.LevelNormal || targetLevel == database.LevelIgnored
	}

	// Normal users cannot manage anyone
	return false
}

// CanAddUserWithLevel checks if an actor can add a user with the specified level
func CanAddUserWithLevel(actorLevel, targetLevel database.PermissionLevel) bool {
	// Owner can add anyone except another owner
	if actorLevel == database.LevelOwner {
		return targetLevel != database.LevelOwner
	}

	// Admin can only add normal and ignored users (not admin or owner)
	if actorLevel == database.LevelAdmin {
		return targetLevel == database.LevelNormal || targetLevel == database.LevelIgnored
	}

	// Normal users cannot add anyone
	return false
}
