package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ChannelUser represents a user in a channel
type ChannelUser struct {
	ID        int64
	Network   string
	Channel   string
	Nick      string
	IsOp      bool
	IsHalfop  bool
	IsVoice   bool
	Hostmask  string
	Account   string
	JoinedAt  time.Time
	UpdatedAt time.Time
}

// BotChannelStatus represents the bot's status in a channel
type BotChannelStatus struct {
	Network    string
	Channel    string
	IsJoined   bool
	IsOp       bool
	IsHalfop   bool
	IsVoice    bool
	UserCount  int
	OpCount    int
	VoiceCount int
	Topic      string
	JoinedAt   *time.Time
	UpdatedAt  time.Time
}

// UpsertChannelUser adds or updates a user in a channel
func (db *DB) UpsertChannelUser(channel, nick string, isOp, isHalfop, isVoice bool) error {
	return db.UpsertChannelUserForNetwork(DefaultNetwork, channel, nick, isOp, isHalfop, isVoice)
}

// UpsertChannelUserForNetwork adds or updates a user in a network channel.
func (db *DB) UpsertChannelUserForNetwork(network, channel, nick string, isOp, isHalfop, isVoice bool) error {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	_, err := db.conn.Exec(`
		INSERT INTO channel_users (network, channel, nick, is_op, is_halfop, is_voice, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(network, channel, nick) DO UPDATE SET
			is_op = excluded.is_op,
			is_halfop = excluded.is_halfop,
			is_voice = excluded.is_voice,
			updated_at = CURRENT_TIMESTAMP
	`, network, channel, nick, isOp, isHalfop, isVoice)

	if err != nil {
		return fmt.Errorf("failed to upsert channel user: %w", err)
	}

	// Update channel counts
	return db.updateChannelCountsForNetwork(network, channel)
}

// TouchChannelUserActivityForNetwork records recent activity for a user in a
// network channel without changing any known mode flags.
func (db *DB) TouchChannelUserActivityForNetwork(network, channel, nick string) error {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)
	nick = strings.TrimSpace(nick)
	if nick == "" {
		return nil
	}

	_, err := db.conn.Exec(`
		INSERT INTO channel_users (network, channel, nick, is_op, is_halfop, is_voice, updated_at)
		VALUES (?, ?, ?, FALSE, FALSE, FALSE, CURRENT_TIMESTAMP)
		ON CONFLICT(network, channel, nick) DO UPDATE SET
			updated_at = CURRENT_TIMESTAMP
	`, network, channel, nick)
	if err != nil {
		return fmt.Errorf("failed to touch channel user activity: %w", err)
	}

	return db.updateChannelCountsForNetwork(network, channel)
}

// ChannelUserEntry represents a user entry for batch operations
type ChannelUserEntry struct {
	Nick     string
	IsOp     bool
	IsHalfop bool
	IsVoice  bool
}

// ReplaceChannelUsersSnapshot replaces all tracked users for a channel with
// a fresh snapshot (typically from IRC NAMES replies).
func (db *DB) ReplaceChannelUsersSnapshot(channel string, users []ChannelUserEntry) error {
	return db.ReplaceChannelUsersSnapshotForNetwork(DefaultNetwork, channel, users)
}

// ReplaceChannelUsersSnapshotForNetwork replaces all tracked users for a network channel.
func (db *DB) ReplaceChannelUsersSnapshotForNetwork(network, channel string, users []ChannelUserEntry) error {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	// Deduplicate nicks case-insensitively in case the upstream snapshot
	// contains repeated entries with different casing.
	uniqueUsers := make(map[string]ChannelUserEntry, len(users))
	for _, u := range users {
		nick := strings.TrimSpace(u.Nick)
		if nick == "" {
			continue
		}
		u.Nick = nick
		uniqueUsers[strings.ToLower(nick)] = u
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Snapshot semantics: clear channel users, then insert current list.
	if _, err := tx.Exec(`DELETE FROM channel_users WHERE network = ? AND channel = ?`, network, channel); err != nil {
		return fmt.Errorf("failed to clear channel users: %w", err)
	}

	if len(uniqueUsers) > 0 {
		stmt, err := tx.Prepare(`
			INSERT INTO channel_users (network, channel, nick, is_op, is_halfop, is_voice, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		`)
		if err != nil {
			return fmt.Errorf("failed to prepare statement: %w", err)
		}
		defer func() { _ = stmt.Close() }()

		for _, u := range uniqueUsers {
			if _, err := stmt.Exec(network, channel, u.Nick, u.IsOp, u.IsHalfop, u.IsVoice); err != nil {
				return fmt.Errorf("failed to insert user %s: %w", u.Nick, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return db.updateChannelCountsForNetwork(network, channel)
}

// BulkUpsertChannelUsers adds or updates multiple users in a channel efficiently
// This uses a transaction to batch all inserts, then updates counts once at the end
func (db *DB) BulkUpsertChannelUsers(channel string, users []ChannelUserEntry) error {
	return db.BulkUpsertChannelUsersForNetwork(DefaultNetwork, channel, users)
}

// BulkUpsertChannelUsersForNetwork adds or updates multiple users in a network channel.
func (db *DB) BulkUpsertChannelUsersForNetwork(network, channel string, users []ChannelUserEntry) error {
	if len(users) == 0 {
		return nil
	}

	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	// Use a transaction for all inserts
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Prepare the statement once
	stmt, err := tx.Prepare(`
		INSERT INTO channel_users (network, channel, nick, is_op, is_halfop, is_voice, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(network, channel, nick) DO UPDATE SET
			is_op = excluded.is_op,
			is_halfop = excluded.is_halfop,
			is_voice = excluded.is_voice,
			updated_at = CURRENT_TIMESTAMP
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	// Insert all users
	for _, u := range users {
		_, err := stmt.Exec(network, channel, u.Nick, u.IsOp, u.IsHalfop, u.IsVoice)
		if err != nil {
			return fmt.Errorf("failed to insert user %s: %w", u.Nick, err)
		}
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Update counts once at the end
	return db.updateChannelCountsForNetwork(network, channel)
}

// RemoveChannelUser removes a user from a channel
func (db *DB) RemoveChannelUser(channel, nick string) error {
	return db.RemoveChannelUserForNetwork(DefaultNetwork, channel, nick)
}

// RemoveChannelUserForNetwork removes a user from a network channel.
func (db *DB) RemoveChannelUserForNetwork(network, channel, nick string) error {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	_, err := db.conn.Exec(`
		DELETE FROM channel_users WHERE network = ? AND channel = ? AND nick = ?
	`, network, channel, nick)

	if err != nil {
		return fmt.Errorf("failed to remove channel user: %w", err)
	}

	// Update channel counts
	return db.updateChannelCountsForNetwork(network, channel)
}

// ClearChannelUsers removes all users from a channel (used on part/kick)
func (db *DB) ClearChannelUsers(channel string) error {
	return db.ClearChannelUsersForNetwork(DefaultNetwork, channel)
}

// ClearChannelUsersForNetwork removes all users from a network channel.
func (db *DB) ClearChannelUsersForNetwork(network, channel string) error {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	_, err := db.conn.Exec(`DELETE FROM channel_users WHERE network = ? AND channel = ?`, network, channel)
	if err != nil {
		return fmt.Errorf("failed to clear channel users: %w", err)
	}

	return nil
}

// UpdateUserMode updates a user's mode in a channel
func (db *DB) UpdateUserMode(channel, nick string, mode string, adding bool) error {
	return db.UpdateUserModeForNetwork(DefaultNetwork, channel, nick, mode, adding)
}

// UpdateUserModeForNetwork updates a user's mode in a network channel.
func (db *DB) UpdateUserModeForNetwork(network, channel, nick string, mode string, adding bool) error {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	var column string
	switch mode {
	case "o":
		column = "is_op"
	case "h":
		column = "is_halfop"
	case "v":
		column = "is_voice"
	default:
		return nil // Ignore unknown modes
	}

	query := fmt.Sprintf(`
		UPDATE channel_users SET %s = ?, updated_at = CURRENT_TIMESTAMP
		WHERE network = ? AND channel = ? AND nick = ?
	`, column)

	_, err := db.conn.Exec(query, adding, network, channel, nick)
	if err != nil {
		return fmt.Errorf("failed to update user mode: %w", err)
	}

	// Update channel counts
	return db.updateChannelCountsForNetwork(network, channel)
}

// RenameChannelUser updates a user's nick in all channels
func (db *DB) RenameChannelUser(oldNick, newNick string) error {
	return db.RenameChannelUserForNetwork(DefaultNetwork, oldNick, newNick)
}

// RenameChannelUserForNetwork updates a user's nick in all channels on a network.
func (db *DB) RenameChannelUserForNetwork(network, oldNick, newNick string) error {
	network = normalizeNetwork(network)
	_, err := db.conn.Exec(`
		UPDATE channel_users SET nick = ?, updated_at = CURRENT_TIMESTAMP
		WHERE network = ? AND nick = ?
	`, newNick, network, oldNick)

	if err != nil {
		return fmt.Errorf("failed to rename channel user: %w", err)
	}

	return nil
}

// GetChannelUsers returns all users in a channel
func (db *DB) GetChannelUsers(channel string) ([]ChannelUser, error) {
	return db.GetChannelUsersForNetwork(DefaultNetwork, channel)
}

// GetChannelUsersForNetwork returns all users in a network channel.
func (db *DB) GetChannelUsersForNetwork(network, channel string) ([]ChannelUser, error) {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	rows, err := db.conn.Query(`
		SELECT id, network, channel, nick, is_op, is_halfop, is_voice, 
		       COALESCE(hostmask, ''), COALESCE(account, ''),
		       joined_at, updated_at
		FROM channel_users WHERE network = ? AND channel = ?
		ORDER BY nick
	`, network, channel)
	if err != nil {
		return nil, fmt.Errorf("failed to query channel users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []ChannelUser
	for rows.Next() {
		var u ChannelUser
		err := rows.Scan(&u.ID, &u.Network, &u.Channel, &u.Nick, &u.IsOp, &u.IsHalfop, &u.IsVoice,
			&u.Hostmask, &u.Account, &u.JoinedAt, &u.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan channel user: %w", err)
		}
		users = append(users, u)
	}

	return users, nil
}

// GetChannelUser returns a specific user in a channel
func (db *DB) GetChannelUser(channel, nick string) (*ChannelUser, error) {
	return db.GetChannelUserForNetwork(DefaultNetwork, channel, nick)
}

// GetChannelUserForNetwork returns a specific user in a network channel.
func (db *DB) GetChannelUserForNetwork(network, channel, nick string) (*ChannelUser, error) {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	var u ChannelUser
	err := db.conn.QueryRow(`
		SELECT id, network, channel, nick, is_op, is_halfop, is_voice,
		       COALESCE(hostmask, ''), COALESCE(account, ''),
		       joined_at, updated_at
		FROM channel_users WHERE network = ? AND channel = ? AND nick = ?
	`, network, channel, nick).Scan(&u.ID, &u.Network, &u.Channel, &u.Nick, &u.IsOp, &u.IsHalfop, &u.IsVoice,
		&u.Hostmask, &u.Account, &u.JoinedAt, &u.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get channel user: %w", err)
	}

	return &u, nil
}

// SetBotChannelStatus updates the bot's status in a channel
func (db *DB) SetBotChannelStatus(channel string, isJoined, isOp, isHalfop, isVoice bool) error {
	return db.SetBotChannelStatusForNetwork(DefaultNetwork, channel, isJoined, isOp, isHalfop, isVoice)
}

// SetBotChannelStatusForNetwork updates the bot's status in a network channel.
func (db *DB) SetBotChannelStatusForNetwork(network, channel string, isJoined, isOp, isHalfop, isVoice bool) error {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	var joinedAt interface{}
	if isJoined {
		joinedAt = time.Now()
	}

	_, err := db.conn.Exec(`
		INSERT INTO bot_channel_status (network, channel, is_joined, is_op, is_halfop, is_voice, joined_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(network, channel) DO UPDATE SET
			is_joined = excluded.is_joined,
			is_op = excluded.is_op,
			is_halfop = excluded.is_halfop,
			is_voice = excluded.is_voice,
			joined_at = CASE WHEN excluded.is_joined AND bot_channel_status.joined_at IS NULL 
			            THEN excluded.joined_at ELSE bot_channel_status.joined_at END,
			updated_at = CURRENT_TIMESTAMP
	`, network, channel, isJoined, isOp, isHalfop, isVoice, joinedAt)

	if err != nil {
		return fmt.Errorf("failed to set bot channel status: %w", err)
	}

	return nil
}

// TouchBotChannelActivityForNetwork records that the bot is actively seeing a
// channel on a network without overwriting known channel modes.
func (db *DB) TouchBotChannelActivityForNetwork(network, channel string) error {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	_, err := db.conn.Exec(`
		INSERT INTO bot_channel_status (network, channel, is_joined, is_op, is_halfop, is_voice, joined_at, updated_at)
		VALUES (?, ?, TRUE, FALSE, FALSE, FALSE, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(network, channel) DO UPDATE SET
			is_joined = TRUE,
			joined_at = CASE WHEN bot_channel_status.joined_at IS NULL
			            THEN excluded.joined_at ELSE bot_channel_status.joined_at END,
			updated_at = CURRENT_TIMESTAMP
	`, network, channel, time.Now())
	if err != nil {
		return fmt.Errorf("failed to touch bot channel activity: %w", err)
	}

	return nil
}

// UpdateBotChannelMode updates a specific mode for the bot in a channel
func (db *DB) UpdateBotChannelMode(channel string, mode string, adding bool) error {
	return db.UpdateBotChannelModeForNetwork(DefaultNetwork, channel, mode, adding)
}

// UpdateBotChannelModeForNetwork updates a specific mode for the bot in a network channel.
func (db *DB) UpdateBotChannelModeForNetwork(network, channel string, mode string, adding bool) error {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	var column string
	switch mode {
	case "o":
		column = "is_op"
	case "h":
		column = "is_halfop"
	case "v":
		column = "is_voice"
	default:
		return nil
	}

	query := fmt.Sprintf(`
		UPDATE bot_channel_status SET %s = ?, updated_at = CURRENT_TIMESTAMP
		WHERE network = ? AND channel = ?
	`, column)

	_, err := db.conn.Exec(query, adding, network, channel)
	if err != nil {
		return fmt.Errorf("failed to update bot channel mode: %w", err)
	}

	return nil
}

// SetBotChannelTopic updates the topic for a channel
func (db *DB) SetBotChannelTopic(channel, topic string) error {
	return db.SetBotChannelTopicForNetwork(DefaultNetwork, channel, topic)
}

// SetBotChannelTopicForNetwork updates the topic for a network channel.
func (db *DB) SetBotChannelTopicForNetwork(network, channel, topic string) error {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	_, err := db.conn.Exec(`
		UPDATE bot_channel_status SET topic = ?, updated_at = CURRENT_TIMESTAMP
		WHERE network = ? AND channel = ?
	`, topic, network, channel)

	if err != nil {
		return fmt.Errorf("failed to set bot channel topic: %w", err)
	}

	return nil
}

// GetBotChannelStatus returns the bot's status in a channel
func (db *DB) GetBotChannelStatus(channel string) (*BotChannelStatus, error) {
	return db.GetBotChannelStatusForNetwork(DefaultNetwork, channel)
}

// GetBotChannelStatusForNetwork returns the bot's status in a network channel.
func (db *DB) GetBotChannelStatusForNetwork(network, channel string) (*BotChannelStatus, error) {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	var s BotChannelStatus
	var joinedAt sql.NullTime
	var topic sql.NullString

	err := db.conn.QueryRow(`
		SELECT network, channel, is_joined, is_op, is_halfop, is_voice,
		       user_count, op_count, voice_count, topic, joined_at, updated_at
		FROM bot_channel_status WHERE network = ? AND channel = ?
	`, network, channel).Scan(&s.Network, &s.Channel, &s.IsJoined, &s.IsOp, &s.IsHalfop, &s.IsVoice,
		&s.UserCount, &s.OpCount, &s.VoiceCount, &topic, &joinedAt, &s.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get bot channel status: %w", err)
	}

	if joinedAt.Valid {
		s.JoinedAt = &joinedAt.Time
	}
	if topic.Valid {
		s.Topic = topic.String
	}

	return &s, nil
}

// GetAllBotChannelStatuses returns the bot's status in all channels
func (db *DB) GetAllBotChannelStatuses() ([]BotChannelStatus, error) {
	return db.GetAllBotChannelStatusesForNetwork(DefaultNetwork)
}

// GetAllBotChannelStatusesForNetwork returns the bot's status in all joined channels on a network.
func (db *DB) GetAllBotChannelStatusesForNetwork(network string) ([]BotChannelStatus, error) {
	network = normalizeNetwork(network)
	rows, err := db.conn.Query(`
		SELECT network, channel, is_joined, is_op, is_halfop, is_voice,
		       user_count, op_count, voice_count, topic, joined_at, updated_at
		FROM bot_channel_status WHERE network = ? AND is_joined = TRUE
		ORDER BY channel
	`, network)
	if err != nil {
		return nil, fmt.Errorf("failed to query bot channel statuses: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var statuses []BotChannelStatus
	for rows.Next() {
		var s BotChannelStatus
		var joinedAt sql.NullTime
		var topic sql.NullString

		err := rows.Scan(&s.Network, &s.Channel, &s.IsJoined, &s.IsOp, &s.IsHalfop, &s.IsVoice,
			&s.UserCount, &s.OpCount, &s.VoiceCount, &topic, &joinedAt, &s.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan bot channel status: %w", err)
		}

		if joinedAt.Valid {
			s.JoinedAt = &joinedAt.Time
		}
		if topic.Valid {
			s.Topic = topic.String
		}

		statuses = append(statuses, s)
	}

	return statuses, nil
}

// BotHasOp returns whether the bot has op in a channel
func (db *DB) BotHasOp(channel string) (bool, error) {
	return db.BotHasOpForNetwork(DefaultNetwork, channel)
}

// BotHasOpForNetwork returns whether the bot has op in a network channel.
func (db *DB) BotHasOpForNetwork(network, channel string) (bool, error) {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	var isOp bool
	err := db.conn.QueryRow(`
		SELECT is_op FROM bot_channel_status WHERE network = ? AND channel = ? AND is_joined = TRUE
	`, network, channel).Scan(&isOp)

	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check bot op status: %w", err)
	}

	return isOp, nil
}

func (db *DB) updateChannelCountsForNetwork(network, channel string) error {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	_, err := db.conn.Exec(`
		UPDATE bot_channel_status SET
			user_count = (SELECT COUNT(*) FROM channel_users WHERE network = ? AND channel = ?),
			op_count = (SELECT COUNT(*) FROM channel_users WHERE network = ? AND channel = ? AND is_op = TRUE),
			voice_count = (SELECT COUNT(*) FROM channel_users WHERE network = ? AND channel = ? AND is_voice = TRUE),
			updated_at = CURRENT_TIMESTAMP
		WHERE network = ? AND channel = ?
	`, network, channel, network, channel, network, channel, network, channel)

	if err != nil {
		return fmt.Errorf("failed to update channel counts: %w", err)
	}

	return nil
}

// MarkBotLeftChannel marks the bot as having left a channel
func (db *DB) MarkBotLeftChannel(channel string) error {
	return db.MarkBotLeftChannelForNetwork(DefaultNetwork, channel)
}

// MarkBotLeftChannelForNetwork marks the bot as having left a network channel.
func (db *DB) MarkBotLeftChannelForNetwork(network, channel string) error {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	// Clear all users from the channel
	if err := db.ClearChannelUsersForNetwork(network, channel); err != nil {
		return err
	}

	// Update bot status
	_, err := db.conn.Exec(`
		UPDATE bot_channel_status SET 
			is_joined = FALSE, is_op = FALSE, is_halfop = FALSE, is_voice = FALSE,
			user_count = 0, op_count = 0, voice_count = 0,
			updated_at = CURRENT_TIMESTAMP
		WHERE network = ? AND channel = ?
	`, network, channel)

	if err != nil {
		return fmt.Errorf("failed to mark bot left channel: %w", err)
	}

	return nil
}

// RemoveUserFromAllChannels removes a user from all channels (used on QUIT)
func (db *DB) RemoveUserFromAllChannels(nick string) error {
	return db.RemoveUserFromAllChannelsForNetwork(DefaultNetwork, nick)
}

// RemoveUserFromAllChannelsForNetwork removes a user from all channels on a network.
func (db *DB) RemoveUserFromAllChannelsForNetwork(network, nick string) error {
	network = normalizeNetwork(network)
	// Get all channels the user is in
	rows, err := db.conn.Query(`SELECT DISTINCT channel FROM channel_users WHERE network = ? AND nick = ?`, network, nick)
	if err != nil {
		return fmt.Errorf("failed to query user channels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var channels []string
	for rows.Next() {
		var channel string
		if err := rows.Scan(&channel); err != nil {
			return fmt.Errorf("failed to scan channel: %w", err)
		}
		channels = append(channels, channel)
	}

	// Delete user from all channels
	_, err = db.conn.Exec(`DELETE FROM channel_users WHERE network = ? AND nick = ?`, network, nick)
	if err != nil {
		return fmt.Errorf("failed to remove user from all channels: %w", err)
	}

	// Update counts for each affected channel
	for _, channel := range channels {
		if err := db.updateChannelCountsForNetwork(network, channel); err != nil {
			// Log but don't fail
			continue
		}
	}

	return nil
}

// GetChannelUsersByMode returns list of nicks with a specific mode in a channel
func (db *DB) GetChannelUsersByMode(channel, mode string) ([]string, error) {
	return db.GetChannelUsersByModeForNetwork(DefaultNetwork, channel, mode)
}

// GetChannelUsersByModeForNetwork returns list of nicks with a specific mode in a network channel.
func (db *DB) GetChannelUsersByModeForNetwork(network, channel, mode string) ([]string, error) {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	var column string
	switch mode {
	case "op":
		column = "is_op"
	case "halfop":
		column = "is_halfop"
	case "voice":
		column = "is_voice"
	default:
		return nil, fmt.Errorf("unknown mode: %s", mode)
	}

	query := fmt.Sprintf(`SELECT nick FROM channel_users WHERE network = ? AND channel = ? AND %s = TRUE ORDER BY nick`, column)
	rows, err := db.conn.Query(query, network, channel)
	if err != nil {
		return nil, fmt.Errorf("failed to query users by mode: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var nicks []string
	for rows.Next() {
		var nick string
		if err := rows.Scan(&nick); err != nil {
			return nil, fmt.Errorf("failed to scan nick: %w", err)
		}
		nicks = append(nicks, nick)
	}

	return nicks, nil
}

// FindUserChannels finds all channels a user is in
func (db *DB) FindUserChannels(nick string) ([]string, error) {
	return db.FindUserChannelsForNetwork(DefaultNetwork, nick)
}

// FindUserChannelsForNetwork finds all channels a user is in on a network.
func (db *DB) FindUserChannelsForNetwork(network, nick string) ([]string, error) {
	network = normalizeNetwork(network)
	rows, err := db.conn.Query(`
		SELECT channel FROM channel_users WHERE network = ? AND nick = ? COLLATE NOCASE ORDER BY channel
	`, network, nick)
	if err != nil {
		return nil, fmt.Errorf("failed to find user channels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var channels []string
	for rows.Next() {
		var channel string
		if err := rows.Scan(&channel); err != nil {
			return nil, fmt.Errorf("failed to scan channel: %w", err)
		}
		channels = append(channels, channel)
	}

	return channels, nil
}

// GetChannelUserNicks returns all nicks in a channel
func (db *DB) GetChannelUserNicks(channel string) ([]string, error) {
	return db.GetChannelUserNicksForNetwork(DefaultNetwork, channel)
}

// GetChannelUserNicksForNetwork returns all nicks in a network channel.
func (db *DB) GetChannelUserNicksForNetwork(network, channel string) ([]string, error) {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	rows, err := db.conn.Query(`
		SELECT nick FROM channel_users WHERE network = ? AND channel = ? ORDER BY nick
	`, network, channel)
	if err != nil {
		return nil, fmt.Errorf("failed to query channel user nicks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var nicks []string
	for rows.Next() {
		var nick string
		if err := rows.Scan(&nick); err != nil {
			return nil, fmt.Errorf("failed to scan nick: %w", err)
		}
		nicks = append(nicks, nick)
	}

	return nicks, nil
}

// GetChannelRegularUsers returns users without op, halfop, or voice in a channel
func (db *DB) GetChannelRegularUsers(channel string) ([]string, error) {
	return db.GetChannelRegularUsersForNetwork(DefaultNetwork, channel)
}

// GetChannelRegularUsersForNetwork returns users without op, halfop, or voice in a network channel.
func (db *DB) GetChannelRegularUsersForNetwork(network, channel string) ([]string, error) {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	rows, err := db.conn.Query(`
		SELECT nick FROM channel_users 
		WHERE network = ? AND channel = ? AND is_op = FALSE AND is_halfop = FALSE AND is_voice = FALSE
		ORDER BY nick
	`, network, channel)
	if err != nil {
		return nil, fmt.Errorf("failed to query regular users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var nicks []string
	for rows.Next() {
		var nick string
		if err := rows.Scan(&nick); err != nil {
			return nil, fmt.Errorf("failed to scan nick: %w", err)
		}
		nicks = append(nicks, nick)
	}

	return nicks, nil
}

// SearchChannelUsers searches for users in a channel by nick pattern (case-insensitive LIKE)
func (db *DB) SearchChannelUsers(channel, pattern string) ([]ChannelUser, error) {
	return db.SearchChannelUsersForNetwork(DefaultNetwork, channel, pattern)
}

// SearchChannelUsersForNetwork searches for users in a network channel.
func (db *DB) SearchChannelUsersForNetwork(network, channel, pattern string) ([]ChannelUser, error) {
	network = normalizeNetwork(network)
	channel = strings.ToLower(channel)

	rows, err := db.conn.Query(`
		SELECT id, network, channel, nick, is_op, is_halfop, is_voice, 
		       COALESCE(hostmask, ''), COALESCE(account, ''),
		       joined_at, updated_at
		FROM channel_users 
		WHERE network = ? AND channel = ? AND nick LIKE ? COLLATE NOCASE
		ORDER BY nick
	`, network, channel, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to search channel users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []ChannelUser
	for rows.Next() {
		var u ChannelUser
		err := rows.Scan(&u.ID, &u.Network, &u.Channel, &u.Nick, &u.IsOp, &u.IsHalfop, &u.IsVoice,
			&u.Hostmask, &u.Account, &u.JoinedAt, &u.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan channel user: %w", err)
		}
		users = append(users, u)
	}

	return users, nil
}

// SearchUsersGlobal searches for users across all channels by nick pattern
func (db *DB) SearchUsersGlobal(pattern string) ([]ChannelUser, error) {
	return db.SearchUsersGlobalForNetwork(DefaultNetwork, pattern)
}

// SearchUsersGlobalForNetwork searches for users across all channels in a network.
func (db *DB) SearchUsersGlobalForNetwork(network, pattern string) ([]ChannelUser, error) {
	network = normalizeNetwork(network)
	rows, err := db.conn.Query(`
		SELECT id, network, channel, nick, is_op, is_halfop, is_voice, 
		       COALESCE(hostmask, ''), COALESCE(account, ''),
		       joined_at, updated_at
		FROM channel_users 
		WHERE network = ? AND nick LIKE ? COLLATE NOCASE
		ORDER BY nick, channel
	`, network, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to search users globally: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []ChannelUser
	for rows.Next() {
		var u ChannelUser
		err := rows.Scan(&u.ID, &u.Network, &u.Channel, &u.Nick, &u.IsOp, &u.IsHalfop, &u.IsVoice,
			&u.Hostmask, &u.Account, &u.JoinedAt, &u.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan channel user: %w", err)
		}
		users = append(users, u)
	}

	return users, nil
}
