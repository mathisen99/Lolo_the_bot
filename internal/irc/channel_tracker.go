// Package irc provides IRC client functionality.
package irc

import (
	"strings"
	"sync"
	"time"

	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/output"
)

// ChannelTracker implements ChannelUserTracker using the database for persistence
type ChannelTracker struct {
	db      *database.DB
	logger  output.Logger
	botNick string

	// Buffering for NAMES replies (353) - IRC sends multiple messages for large channels
	namesBuf   map[string][]database.ChannelUserEntry // channel -> pending users
	namesMu    sync.Mutex
	namesTimer map[string]*time.Timer // channel -> flush timer
}

// NewChannelTracker creates a new channel tracker
func NewChannelTracker(db *database.DB, logger output.Logger, botNick string) *ChannelTracker {
	return &ChannelTracker{
		db:         db,
		logger:     logger,
		botNick:    botNick,
		namesBuf:   make(map[string][]database.ChannelUserEntry),
		namesTimer: make(map[string]*time.Timer),
	}
}

// SetBotNick updates the bot's current nickname (for tracking mode changes)
func (ct *ChannelTracker) SetBotNick(nick string) {
	ct.botNick = nick
}

// OnNamesReply handles RPL_NAMREPLY (353) - buffers users for batch insert
// Names format: @nick (op), +nick (voice), %nick (halfop), nick (regular)
// IRC sends multiple 353 messages for large channels, followed by 366 (end of names)
func (ct *ChannelTracker) OnNamesReply(channel string, names []string) {
	ct.namesMu.Lock()
	defer ct.namesMu.Unlock()

	channelLower := strings.ToLower(channel)

	// Parse and buffer users
	for _, name := range names {
		if name == "" {
			continue
		}

		// Parse prefix to determine mode
		isOp := false
		isHalfop := false
		isVoice := false
		nick := name

		// Handle mode prefixes (can be multiple, e.g., @+ for op+voice)
		for len(nick) > 0 {
			switch nick[0] {
			case '@':
				isOp = true
				nick = nick[1:]
			case '%':
				isHalfop = true
				nick = nick[1:]
			case '+':
				isVoice = true
				nick = nick[1:]
			default:
				goto done
			}
		}
	done:

		if nick == "" {
			continue
		}

		// Buffer the user
		ct.namesBuf[channelLower] = append(ct.namesBuf[channelLower], database.ChannelUserEntry{
			Nick:     nick,
			IsOp:     isOp,
			IsHalfop: isHalfop,
			IsVoice:  isVoice,
		})

		// Track bot status separately (we'll update it during flush)
		if strings.EqualFold(nick, ct.botNick) {
			// Update bot status immediately so we know our op status
			if err := ct.db.SetBotChannelStatus(channelLower, true, isOp, isHalfop, isVoice); err != nil {
				ct.logger.Warning("Failed to set bot channel status for %s: %v", channel, err)
			}
		}
	}

	// Reset/start the flush timer - wait 500ms after last 353 before flushing
	// This handles the case where IRC sends many 353 messages in quick succession
	if timer, exists := ct.namesTimer[channelLower]; exists {
		timer.Stop()
	}
	ct.namesTimer[channelLower] = time.AfterFunc(500*time.Millisecond, func() {
		ct.flushNamesBuffer(channelLower)
	})
}

// flushNamesBuffer writes buffered users to database
func (ct *ChannelTracker) flushNamesBuffer(channel string) {
	ct.namesMu.Lock()
	users := ct.namesBuf[channel]
	delete(ct.namesBuf, channel)
	delete(ct.namesTimer, channel)
	ct.namesMu.Unlock()

	if len(users) == 0 {
		return
	}

	ct.logger.Info("Flushing %d users for %s to database", len(users), channel)

	// Batch insert all users
	if err := ct.db.BulkUpsertChannelUsers(channel, users); err != nil {
		ct.logger.Warning("Failed to bulk upsert users for %s: %v", channel, err)
	}
}

// OnJoin handles a user joining a channel
func (ct *ChannelTracker) OnJoin(channel, nick string, isSelf bool) {
	if isSelf {
		// Bot joined - set initial status (no modes yet)
		if err := ct.db.SetBotChannelStatus(channel, true, false, false, false); err != nil {
			ct.logger.Warning("Failed to set bot channel status for %s: %v", channel, err)
		}
	}

	// Add user to channel (no modes on join)
	if err := ct.db.UpsertChannelUser(channel, nick, false, false, false); err != nil {
		ct.logger.Warning("Failed to add user %s to %s: %v", nick, channel, err)
	}
}

// OnPart handles a user leaving a channel
func (ct *ChannelTracker) OnPart(channel, nick string, isSelf bool) {
	if isSelf {
		// Bot left - clear all channel data
		if err := ct.db.MarkBotLeftChannel(channel); err != nil {
			ct.logger.Warning("Failed to mark bot left %s: %v", channel, err)
		}
	} else {
		// Remove user from channel
		if err := ct.db.RemoveChannelUser(channel, nick); err != nil {
			ct.logger.Warning("Failed to remove user %s from %s: %v", nick, channel, err)
		}
	}
}

// OnQuit handles a user quitting IRC (leaves all channels)
func (ct *ChannelTracker) OnQuit(nick string) {
	// Get all channels and remove user from each
	// For simplicity, we use a direct query to remove from all channels
	if err := ct.db.RemoveUserFromAllChannels(nick); err != nil {
		ct.logger.Warning("Failed to remove user %s from all channels: %v", nick, err)
	}
}

// OnKick handles a user being kicked from a channel
func (ct *ChannelTracker) OnKick(channel, nick string, isSelf bool) {
	if isSelf {
		// Bot was kicked - clear all channel data
		if err := ct.db.MarkBotLeftChannel(channel); err != nil {
			ct.logger.Warning("Failed to mark bot left %s: %v", channel, err)
		}
	} else {
		// Remove user from channel
		if err := ct.db.RemoveChannelUser(channel, nick); err != nil {
			ct.logger.Warning("Failed to remove kicked user %s from %s: %v", nick, channel, err)
		}
	}
}

// OnMode handles mode changes for a user in a channel
func (ct *ChannelTracker) OnMode(channel, nick, mode string, adding bool, isSelf bool) {
	// Update user mode in database
	if err := ct.db.UpdateUserMode(channel, nick, mode, adding); err != nil {
		ct.logger.Warning("Failed to update mode %s for %s in %s: %v", mode, nick, channel, err)
	}

	// If this affects the bot, update bot status
	if isSelf {
		if err := ct.db.UpdateBotChannelMode(channel, mode, adding); err != nil {
			ct.logger.Warning("Failed to update bot mode %s in %s: %v", mode, channel, err)
		}
	}
}

// OnNickChange handles a user changing their nickname
func (ct *ChannelTracker) OnNickChange(oldNick, newNick string, isSelf bool) {
	// Update nick in all channels
	if err := ct.db.RenameChannelUser(oldNick, newNick); err != nil {
		ct.logger.Warning("Failed to rename user %s to %s: %v", oldNick, newNick, err)
	}

	// Update bot nick reference if it's us
	if isSelf {
		ct.botNick = newNick
	}
}

// OnTopic handles topic changes
func (ct *ChannelTracker) OnTopic(channel, topic string) {
	if err := ct.db.SetBotChannelTopic(channel, topic); err != nil {
		ct.logger.Warning("Failed to set topic for %s: %v", channel, err)
	}
}
