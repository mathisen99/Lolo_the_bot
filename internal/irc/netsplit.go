package irc

import (
	"sync"
	"time"

	"github.com/yourusername/lolo/internal/output"
	"github.com/yourusername/lolo/internal/user"
)

// NetsplitDetector detects netsplit events (mass quit/join patterns)
// A netsplit is detected when:
// - Multiple users quit within a short time window (typically 5-10 seconds)
// - Followed by multiple users rejoining
type NetsplitDetector struct {
	mu                  sync.RWMutex
	logger              output.Logger
	userManager         *user.Manager
	quitWindow          time.Duration
	rejoinWindow        time.Duration
	minUsersForNetsplit int
	recentQuits         map[string]time.Time // nick -> quit time
	recentJoins         map[string]time.Time // nick -> join time
	netsplitInProgress  bool
	lastNetsplitTime    time.Time
	netsplitCooldown    time.Duration
	affectedUsers       []string         // Users affected by current netsplit
	netsplitCallbacks   []func([]string) // Callbacks when netsplit is detected
}

// NewNetsplitDetector creates a new netsplit detector
func NewNetsplitDetector(logger output.Logger, userManager *user.Manager) *NetsplitDetector {
	nd := &NetsplitDetector{
		logger:              logger,
		userManager:         userManager,
		quitWindow:          5 * time.Second,  // 5 second window for detecting mass quits
		rejoinWindow:        10 * time.Second, // 10 second window for detecting mass rejoins
		minUsersForNetsplit: 3,                // At least 3 users for it to be considered a netsplit
		recentQuits:         make(map[string]time.Time),
		recentJoins:         make(map[string]time.Time),
		netsplitCooldown:    30 * time.Second, // Don't detect another netsplit for 30 seconds
		affectedUsers:       []string{},
		netsplitCallbacks:   []func([]string){},
	}

	// Start cleanup goroutine to remove old entries
	go nd.cleanupOldEntries()

	return nd
}

// OnQuit records a user quit event
func (nd *NetsplitDetector) OnQuit(nick string) {
	nd.mu.Lock()
	defer nd.mu.Unlock()

	// Only track registered users
	isRegistered, err := nd.userManager.IsRegisteredUser(nick)
	if err != nil || !isRegistered {
		return
	}

	nd.recentQuits[nick] = time.Now()

	// Check if we have a mass quit pattern
	nd.checkForNetsplit()
}

// OnJoin records a user join event
func (nd *NetsplitDetector) OnJoin(nick string) {
	nd.mu.Lock()
	defer nd.mu.Unlock()

	// Only track registered users
	isRegistered, err := nd.userManager.IsRegisteredUser(nick)
	if err != nil || !isRegistered {
		return
	}

	nd.recentJoins[nick] = time.Now()

	// Check if we have a mass rejoin pattern (netsplit recovery)
	nd.checkForNetsplitRecovery()
}

// checkForNetsplit checks if we have a mass quit pattern indicating a netsplit
func (nd *NetsplitDetector) checkForNetsplit() {
	// Don't detect another netsplit if one was recently detected
	if time.Since(nd.lastNetsplitTime) < nd.netsplitCooldown {
		return
	}

	now := time.Now()
	recentQuitCount := 0
	var quitNicks []string

	// Count quits within the window
	for nick, quitTime := range nd.recentQuits {
		if now.Sub(quitTime) < nd.quitWindow {
			recentQuitCount++
			quitNicks = append(quitNicks, nick)
		}
	}

	// If we have enough quits, it's likely a netsplit
	if recentQuitCount >= nd.minUsersForNetsplit {
		nd.logger.Warning("Netsplit detected: %d users quit within %v", recentQuitCount, nd.quitWindow)
		nd.netsplitInProgress = true
		nd.lastNetsplitTime = now
		nd.affectedUsers = quitNicks

		// Trigger callbacks
		for _, callback := range nd.netsplitCallbacks {
			callback(quitNicks)
		}
	}
}

// checkForNetsplitRecovery checks if we have a mass rejoin pattern (netsplit recovery)
func (nd *NetsplitDetector) checkForNetsplitRecovery() {
	now := time.Now()
	recentJoinCount := 0

	// Count joins within the window
	for _, joinTime := range nd.recentJoins {
		if now.Sub(joinTime) < nd.rejoinWindow {
			recentJoinCount++
		}
	}

	// If we have enough rejoins and a netsplit was in progress, it's likely recovery
	if nd.netsplitInProgress && recentJoinCount >= nd.minUsersForNetsplit {
		nd.logger.Info("Netsplit recovery detected: %d users rejoined within %v", recentJoinCount, nd.rejoinWindow)
		nd.netsplitInProgress = false

		// Refresh WHOIS cache for affected users
		nd.refreshAffectedUsers()
	}
}

// refreshAffectedUsers refreshes WHOIS cache entries for users affected by the netsplit
func (nd *NetsplitDetector) refreshAffectedUsers() {
	if len(nd.affectedUsers) == 0 {
		return
	}

	nd.logger.Info("Refreshing WHOIS cache for %d users affected by netsplit", len(nd.affectedUsers))

	// Invalidate cache entries for affected users so they'll be re-WHOIS'd on next permission check
	nd.userManager.InvalidateMultipleWhoisCache(nd.affectedUsers)

	nd.affectedUsers = []string{}
}

// RegisterCallback registers a callback to be called when a netsplit is detected
func (nd *NetsplitDetector) RegisterCallback(callback func([]string)) {
	nd.mu.Lock()
	defer nd.mu.Unlock()

	nd.netsplitCallbacks = append(nd.netsplitCallbacks, callback)
}

// cleanupOldEntries periodically removes old entries from the tracking maps
func (nd *NetsplitDetector) cleanupOldEntries() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		nd.mu.Lock()

		now := time.Now()

		// Remove quits older than 2x the quit window
		for nick, quitTime := range nd.recentQuits {
			if now.Sub(quitTime) > 2*nd.quitWindow {
				delete(nd.recentQuits, nick)
			}
		}

		// Remove joins older than 2x the rejoin window
		for nick, joinTime := range nd.recentJoins {
			if now.Sub(joinTime) > 2*nd.rejoinWindow {
				delete(nd.recentJoins, nick)
			}
		}

		nd.mu.Unlock()
	}
}

// GetNetsplitStatus returns whether a netsplit is currently in progress
func (nd *NetsplitDetector) GetNetsplitStatus() bool {
	nd.mu.RLock()
	defer nd.mu.RUnlock()

	return nd.netsplitInProgress
}

// GetAffectedUsers returns the list of users affected by the current netsplit
func (nd *NetsplitDetector) GetAffectedUsers() []string {
	nd.mu.RLock()
	defer nd.mu.RUnlock()

	// Return a copy to avoid race conditions
	result := make([]string, len(nd.affectedUsers))
	copy(result, nd.affectedUsers)
	return result
}

// GetRecentQuitCount returns the number of recent quits
func (nd *NetsplitDetector) GetRecentQuitCount() int {
	nd.mu.RLock()
	defer nd.mu.RUnlock()

	now := time.Now()
	count := 0

	for _, quitTime := range nd.recentQuits {
		if now.Sub(quitTime) < nd.quitWindow {
			count++
		}
	}

	return count
}

// GetRecentJoinCount returns the number of recent joins
func (nd *NetsplitDetector) GetRecentJoinCount() int {
	nd.mu.RLock()
	defer nd.mu.RUnlock()

	now := time.Now()
	count := 0

	for _, joinTime := range nd.recentJoins {
		if now.Sub(joinTime) < nd.rejoinWindow {
			count++
		}
	}

	return count
}
