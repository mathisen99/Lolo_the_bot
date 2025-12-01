package user

import (
	"sync"
	"time"
)

// WhoisCache represents an in-memory cache for WHOIS data
type WhoisCache struct {
	mu      sync.RWMutex
	entries map[string]*WhoisEntry
	ttl     time.Duration
}

// WhoisEntry represents a cached WHOIS entry
type WhoisEntry struct {
	Nick      string
	Hostmask  string
	Timestamp time.Time
}

// NewWhoisCache creates a new WHOIS cache with the specified TTL
func NewWhoisCache(ttl time.Duration) *WhoisCache {
	return &WhoisCache{
		entries: make(map[string]*WhoisEntry),
		ttl:     ttl,
	}
}

// Get retrieves a WHOIS entry from the cache
// Returns nil if the entry doesn't exist or has expired
func (c *WhoisCache) Get(nick string) *WhoisEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[nick]
	if !exists {
		return nil
	}

	// Check if entry has expired
	if time.Since(entry.Timestamp) > c.ttl {
		return nil
	}

	return entry
}

// Set stores a WHOIS entry in the cache
func (c *WhoisCache) Set(nick, hostmask string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[nick] = &WhoisEntry{
		Nick:      nick,
		Hostmask:  hostmask,
		Timestamp: time.Now(),
	}
}

// Delete removes a WHOIS entry from the cache
func (c *WhoisCache) Delete(nick string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, nick)
}

// Clear removes all entries from the cache
func (c *WhoisCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*WhoisEntry)
}

// IsExpired checks if a cache entry has expired
func (c *WhoisCache) IsExpired(nick string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[nick]
	if !exists {
		return true
	}

	return time.Since(entry.Timestamp) > c.ttl
}

// Refresh updates the timestamp of an existing entry
func (c *WhoisCache) Refresh(nick string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.entries[nick]; exists {
		entry.Timestamp = time.Now()
	}
}

// InvalidateMultiple removes multiple entries from the cache
// This is useful for handling netsplits
func (c *WhoisCache) InvalidateMultiple(nicks []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, nick := range nicks {
		delete(c.entries, nick)
	}
}

// GetAll returns all cached entries (for debugging/monitoring)
func (c *WhoisCache) GetAll() map[string]*WhoisEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Create a copy to avoid race conditions
	result := make(map[string]*WhoisEntry, len(c.entries))
	for k, v := range c.entries {
		result[k] = &WhoisEntry{
			Nick:      v.Nick,
			Hostmask:  v.Hostmask,
			Timestamp: v.Timestamp,
		}
	}

	return result
}

// Size returns the number of entries in the cache
func (c *WhoisCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.entries)
}

// CleanupExpired removes all expired entries from the cache
// This should be called periodically to prevent memory leaks
func (c *WhoisCache) CleanupExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := 0
	now := time.Now()

	for nick, entry := range c.entries {
		if now.Sub(entry.Timestamp) > c.ttl {
			delete(c.entries, nick)
			removed++
		}
	}

	return removed
}
