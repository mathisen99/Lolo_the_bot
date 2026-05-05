package database

import (
	"database/sql"
	"fmt"
	"time"
)

// ChannelState represents the enabled/disabled state of a channel
type ChannelState struct {
	Network       string
	Channel       string
	Enabled       bool
	CommandPrefix string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// GetChannelState retrieves the enabled/disabled state for a channel
func (db *DB) GetChannelState(channel string) (bool, error) {
	return db.GetChannelStateForNetwork(DefaultNetwork, channel)
}

// GetChannelStateForNetwork retrieves the enabled/disabled state for a channel on a network.
func (db *DB) GetChannelStateForNetwork(network, channel string) (bool, error) {
	network = normalizeNetwork(network)
	var enabled bool
	query := `SELECT enabled FROM channel_states WHERE network = ? AND channel = ?`
	err := db.conn.QueryRow(query, network, channel).Scan(&enabled)
	if err == sql.ErrNoRows {
		// Default to enabled if not found
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get channel state: %w", err)
	}
	return enabled, nil
}

// SetChannelState sets the enabled/disabled state for a channel
func (db *DB) SetChannelState(channel string, enabled bool) error {
	return db.SetChannelStateForNetwork(DefaultNetwork, channel, enabled)
}

// SetChannelStateForNetwork sets the enabled/disabled state for a network channel.
func (db *DB) SetChannelStateForNetwork(network, channel string, enabled bool) error {
	network = normalizeNetwork(network)
	query := `
		INSERT INTO channel_states (network, channel, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(network, channel) DO UPDATE SET
			enabled = excluded.enabled,
			updated_at = excluded.updated_at
	`
	_, err := db.conn.Exec(query, network, channel, enabled, time.Now(), time.Now())
	if err != nil {
		return fmt.Errorf("failed to set channel state: %w", err)
	}
	return nil
}

// ListChannelStates returns all channel states
func (db *DB) ListChannelStates() ([]*ChannelState, error) {
	return db.ListChannelStatesForNetwork(DefaultNetwork)
}

// ListChannelStatesForNetwork returns all channel states for a network.
func (db *DB) ListChannelStatesForNetwork(network string) ([]*ChannelState, error) {
	network = normalizeNetwork(network)
	query := `
		SELECT network, channel, enabled, COALESCE(command_prefix, ''), created_at, updated_at
		FROM channel_states
		WHERE network = ?
		ORDER BY channel ASC
	`
	rows, err := db.conn.Query(query, network)
	if err != nil {
		return nil, fmt.Errorf("failed to list channel states: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var states []*ChannelState
	for rows.Next() {
		state := &ChannelState{}
		err := rows.Scan(
			&state.Network,
			&state.Channel,
			&state.Enabled,
			&state.CommandPrefix,
			&state.CreatedAt,
			&state.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan channel state: %w", err)
		}
		states = append(states, state)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating channel states: %w", err)
	}

	return states, nil
}

// GetChannelCommandPrefix retrieves a channel-specific command prefix override.
// Returns an empty string when the channel uses the default prefix.
func (db *DB) GetChannelCommandPrefix(channel string) (string, error) {
	return db.GetChannelCommandPrefixForNetwork(DefaultNetwork, channel)
}

// GetChannelCommandPrefixForNetwork retrieves a network/channel-specific command prefix override.
func (db *DB) GetChannelCommandPrefixForNetwork(network, channel string) (string, error) {
	network = normalizeNetwork(network)
	var prefix sql.NullString
	query := `SELECT command_prefix FROM channel_states WHERE network = ? AND channel = ?`
	err := db.conn.QueryRow(query, network, channel).Scan(&prefix)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get channel command prefix: %w", err)
	}
	if !prefix.Valid {
		return "", nil
	}
	return prefix.String, nil
}

// SetChannelCommandPrefix stores a channel-specific command prefix override.
func (db *DB) SetChannelCommandPrefix(channel, prefix string) error {
	return db.SetChannelCommandPrefixForNetwork(DefaultNetwork, channel, prefix)
}

// SetChannelCommandPrefixForNetwork stores a network/channel-specific command prefix override.
func (db *DB) SetChannelCommandPrefixForNetwork(network, channel, prefix string) error {
	network = normalizeNetwork(network)
	query := `
		INSERT INTO channel_states (network, channel, enabled, command_prefix, created_at, updated_at)
		VALUES (?, ?, 1, ?, ?, ?)
		ON CONFLICT(network, channel) DO UPDATE SET
			command_prefix = excluded.command_prefix,
			updated_at = excluded.updated_at
	`
	now := time.Now()
	_, err := db.conn.Exec(query, network, channel, prefix, now, now)
	if err != nil {
		return fmt.Errorf("failed to set channel command prefix: %w", err)
	}
	return nil
}

// ClearChannelCommandPrefix removes a channel-specific command prefix override.
func (db *DB) ClearChannelCommandPrefix(channel string) error {
	return db.ClearChannelCommandPrefixForNetwork(DefaultNetwork, channel)
}

// ClearChannelCommandPrefixForNetwork removes a network/channel-specific command prefix override.
func (db *DB) ClearChannelCommandPrefixForNetwork(network, channel string) error {
	network = normalizeNetwork(network)
	query := `
		INSERT INTO channel_states (network, channel, enabled, command_prefix, created_at, updated_at)
		VALUES (?, ?, 1, NULL, ?, ?)
		ON CONFLICT(network, channel) DO UPDATE SET
			command_prefix = NULL,
			updated_at = excluded.updated_at
	`
	now := time.Now()
	_, err := db.conn.Exec(query, network, channel, now, now)
	if err != nil {
		return fmt.Errorf("failed to clear channel command prefix: %w", err)
	}
	return nil
}

// ListChannelCommandPrefixes returns all explicit per-channel command prefix overrides.
func (db *DB) ListChannelCommandPrefixes() (map[string]string, error) {
	return db.ListChannelCommandPrefixesForNetwork(DefaultNetwork)
}

// ListChannelCommandPrefixesForNetwork returns explicit prefix overrides for one network.
func (db *DB) ListChannelCommandPrefixesForNetwork(network string) (map[string]string, error) {
	network = normalizeNetwork(network)
	rows, err := db.conn.Query(`
		SELECT channel, command_prefix
		FROM channel_states
		WHERE network = ? AND command_prefix IS NOT NULL AND command_prefix != ''
		ORDER BY channel ASC
	`, network)
	if err != nil {
		return nil, fmt.Errorf("failed to list channel command prefixes: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	prefixes := make(map[string]string)
	for rows.Next() {
		var channel string
		var prefix string
		if err := rows.Scan(&channel, &prefix); err != nil {
			return nil, fmt.Errorf("failed to scan channel command prefix: %w", err)
		}
		prefixes[channel] = prefix
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating channel command prefixes: %w", err)
	}

	return prefixes, nil
}

// GetSetting retrieves a bot setting by key
func (db *DB) GetSetting(key string) (string, error) {
	var value string
	query := `SELECT value FROM bot_settings WHERE key = ?`
	err := db.conn.QueryRow(query, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get setting: %w", err)
	}
	return value, nil
}

// SetSetting sets a bot setting
func (db *DB) SetSetting(key, value string) error {
	query := `
		INSERT INTO bot_settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at
	`
	_, err := db.conn.Exec(query, key, value, time.Now())
	if err != nil {
		return fmt.Errorf("failed to set setting: %w", err)
	}
	return nil
}

// GetPMState retrieves the PM enabled/disabled state
func (db *DB) GetPMState() (bool, error) {
	value, err := db.GetSetting("pm_enabled")
	if err != nil {
		return false, err
	}
	if value == "" {
		// Default to enabled
		return true, nil
	}
	return value == "true", nil
}

// SetPMState sets the PM enabled/disabled state
func (db *DB) SetPMState(enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	return db.SetSetting("pm_enabled", value)
}
