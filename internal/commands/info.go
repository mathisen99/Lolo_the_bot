package commands

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/yourusername/lolo/internal/database"
	"gopkg.in/irc.v4"
)

const (
	// BotVersion is the current version of the bot
	BotVersion = "1.0.0"
)

// InfoCommands holds shared state for info commands
type InfoCommands struct {
	startTime   time.Time
	apiEndpoint string
	apiClient   APIHealthChecker
}

// APIHealthChecker is an interface for checking API health and circuit breaker state
type APIHealthChecker interface {
	CheckHealth(ctx context.Context) error
	GetCircuitBreakerState() string
	GetCircuitBreakerStats() CircuitBreakerStats
}

// CircuitBreakerStats holds circuit breaker statistics
type CircuitBreakerStats struct {
	State               string
	ConsecutiveFailures int
	LastFailureTime     time.Time
	LastStateChange     time.Time
}

// NewInfoCommands creates a new info commands handler
func NewInfoCommands(apiEndpoint string, apiClient APIHealthChecker) *InfoCommands {
	return &InfoCommands{
		startTime:   time.Now(),
		apiEndpoint: apiEndpoint,
		apiClient:   apiClient,
	}
}

// VersionCommand implements the !version command
type VersionCommand struct {
	info *InfoCommands
}

// NewVersionCommand creates a new version command
func NewVersionCommand(info *InfoCommands) *VersionCommand {
	return &VersionCommand{info: info}
}

// Name returns the command name
func (c *VersionCommand) Name() string {
	return "version"
}

// Execute runs the version command
func (c *VersionCommand) Execute(ctx *Context) (*Response, error) {
	message := fmt.Sprintf("Lolo IRC Bot v%s", BotVersion)
	return NewResponse(message), nil
}

// RequiredPermission returns the minimum permission level needed
func (c *VersionCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelNormal
}

// Help returns help text for this command
func (c *VersionCommand) Help() string {
	return "!version - Display the bot version"
}

// CooldownDuration returns the cooldown duration for this command
func (c *VersionCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for version command
}

// UptimeCommand implements the !uptime command
type UptimeCommand struct {
	info *InfoCommands
}

// NewUptimeCommand creates a new uptime command
func NewUptimeCommand(info *InfoCommands) *UptimeCommand {
	return &UptimeCommand{info: info}
}

// Name returns the command name
func (c *UptimeCommand) Name() string {
	return "uptime"
}

// Execute runs the uptime command
func (c *UptimeCommand) Execute(ctx *Context) (*Response, error) {
	uptime := time.Since(c.info.startTime)
	message := formatUptime(uptime)
	return NewResponse(message), nil
}

// RequiredPermission returns the minimum permission level needed
func (c *UptimeCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelNormal
}

// Help returns help text for this command
func (c *UptimeCommand) Help() string {
	return "!uptime - Display how long the bot has been running"
}

// CooldownDuration returns the cooldown duration for this command
func (c *UptimeCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for uptime command
}

// StatusCommand implements the !status command
type StatusCommand struct {
	info *InfoCommands
}

// NewStatusCommand creates a new status command
func NewStatusCommand(info *InfoCommands) *StatusCommand {
	return &StatusCommand{info: info}
}

// Name returns the command name
func (c *StatusCommand) Name() string {
	return "status"
}

// Execute runs the status command
func (c *StatusCommand) Execute(ctx *Context) (*Response, error) {
	uptime := time.Since(c.info.startTime)
	uptimeStr := formatUptime(uptime)

	// Check API health and circuit breaker state
	apiStatus := c.checkAPIHealth()
	circuitState := "N/A"
	if c.info.apiClient != nil {
		circuitState = c.info.apiClient.GetCircuitBreakerState()
	}

	message := fmt.Sprintf("Bot Status: Online | Uptime: %s | API: %s | Circuit: %s", uptimeStr, apiStatus, circuitState)
	return NewResponse(message), nil
}

// RequiredPermission returns the minimum permission level needed
func (c *StatusCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelNormal
}

// Help returns help text for this command
func (c *StatusCommand) Help() string {
	return "!status - Display bot status including uptime and API health"
}

// CooldownDuration returns the cooldown duration for this command
func (c *StatusCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for status command
}

// checkAPIHealth checks if the Python API is reachable
func (c *StatusCommand) checkAPIHealth() string {
	if c.info.apiEndpoint == "" {
		return "Not configured"
	}

	// Use the API client if available
	if c.info.apiClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := c.info.apiClient.CheckHealth(ctx)
		if err != nil {
			return "Unreachable"
		}
		return "Healthy"
	}

	// Fallback to direct HTTP check
	healthURL := c.info.apiEndpoint + "/health"
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(healthURL)
	if err != nil {
		return "Unreachable"
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusOK {
		return "Healthy"
	}

	return fmt.Sprintf("Unhealthy (HTTP %d)", resp.StatusCode)
}

// PingCommand implements the !ping command
type PingCommand struct{}

// NewPingCommand creates a new ping command
func NewPingCommand() *PingCommand {
	return &PingCommand{}
}

// Name returns the command name
func (c *PingCommand) Name() string {
	return "ping"
}

// Execute runs the ping command
func (c *PingCommand) Execute(ctx *Context) (*Response, error) {
	return NewResponse("Pong!"), nil
}

// RequiredPermission returns the minimum permission level needed
func (c *PingCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelNormal
}

// Help returns help text for this command
func (c *PingCommand) Help() string {
	return "!ping - Simple ping-pong response to test bot responsiveness"
}

// CooldownDuration returns the cooldown duration for this command
func (c *PingCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for ping command
}

// HelpCommand implements the !help command
type HelpCommand struct {
	registry *Registry
}

// NewHelpCommand creates a new help command
func NewHelpCommand(registry *Registry) *HelpCommand {
	return &HelpCommand{registry: registry}
}

// Name returns the command name
func (c *HelpCommand) Name() string {
	return "help"
}

// Execute runs the help command
func (c *HelpCommand) Execute(ctx *Context) (*Response, error) {
	// If no arguments, list all commands
	if len(ctx.Args) == 0 {
		return c.listCommands(ctx), nil
	}

	// If argument provided, show help for specific command
	// Support multi-word commands (e.g., "user add")
	cmdName := ctx.Args[0]

	// Try two-word command first if we have enough args
	if len(ctx.Args) >= 2 {
		twoWordCmd := cmdName + " " + ctx.Args[1]
		if cmd, exists := c.registry.Get(twoWordCmd); exists {
			helpText := cmd.Help()
			requiredLevel := cmd.RequiredPermission()
			permName := PermissionLevelName(requiredLevel)
			message := fmt.Sprintf("%s (requires %s level)", helpText, permName)
			return NewResponse(message), nil
		}
	}

	// Try single-word command
	cmd, exists := c.registry.Get(cmdName)
	if !exists {
		return NewErrorResponse(fmt.Sprintf("Unknown command: %s", cmdName)), nil
	}

	// Show detailed help for the command
	helpText := cmd.Help()
	requiredLevel := cmd.RequiredPermission()
	permName := PermissionLevelName(requiredLevel)

	message := fmt.Sprintf("%s (requires %s level)", helpText, permName)
	return NewResponse(message), nil
}

// listCommands returns a list of all available commands
func (c *HelpCommand) listCommands(ctx *Context) *Response {
	cmds := c.registry.GetAll()
	if len(cmds) == 0 {
		return NewResponse("No commands available.")
	}

	// Group commands by permission level
	ownerCmds := []string{}
	adminCmds := []string{}
	normalCmds := []string{}

	for _, cmd := range cmds {
		// Only show commands the user has permission to use
		if !HasPermission(ctx.UserLevel, cmd.RequiredPermission()) {
			continue
		}

		cmdName := cmd.Name()
		switch cmd.RequiredPermission() {
		case database.LevelOwner:
			ownerCmds = append(ownerCmds, cmdName)
		case database.LevelAdmin:
			adminCmds = append(adminCmds, cmdName)
		case database.LevelNormal:
			normalCmds = append(normalCmds, cmdName)
		}
	}

	// Build response message
	var message string
	if len(normalCmds) > 0 {
		message += fmt.Sprintf("Available commands: %s", joinCommands(normalCmds))
	}
	if len(adminCmds) > 0 {
		if message != "" {
			message += " | "
		}
		message += fmt.Sprintf("Admin: %s", joinCommands(adminCmds))
	}
	if len(ownerCmds) > 0 {
		if message != "" {
			message += " | "
		}
		message += fmt.Sprintf("Owner: %s", joinCommands(ownerCmds))
	}

	if message == "" {
		message = "No commands available for your permission level."
	} else {
		message += " | Use !help <command> for details"
	}

	return NewResponse(message)
}

// joinCommands joins command names with commas
func joinCommands(cmds []string) string {
	result := ""
	for i, cmd := range cmds {
		if i > 0 {
			result += ", "
		}
		result += cmd
	}
	return result
}

// RequiredPermission returns the minimum permission level needed
func (c *HelpCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelNormal
}

// Help returns help text for this command
func (c *HelpCommand) Help() string {
	return "!help [command] - List all commands or show help for a specific command"
}

// CooldownDuration returns the cooldown duration for this command
func (c *HelpCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for help command
}

// WhoisCommand implements the !whois command
type WhoisCommand struct {
	ircClient IRCClient
}

// NewWhoisCommand creates a new whois command
func NewWhoisCommand(ircClient IRCClient) *WhoisCommand {
	return &WhoisCommand{ircClient: ircClient}
}

// Name returns the command name
func (c *WhoisCommand) Name() string {
	return "whois"
}

// Execute runs the whois command
func (c *WhoisCommand) Execute(ctx *Context) (*Response, error) {
	if len(ctx.Args) == 0 {
		return NewErrorResponse("Usage: !whois <nickname>"), nil
	}

	nick := ctx.Args[0]

	// Send WHOIS command to IRC server
	// Note: The response will come through IRC message handlers, not directly here
	// This is just triggering the WHOIS request
	msg := &irc.Message{
		Command: "WHOIS",
		Params:  []string{nick},
	}

	if err := c.ircClient.Write(msg); err != nil {
		return NewErrorResponse(fmt.Sprintf("Failed to send WHOIS request: %v", err)), nil
	}

	return NewResponse(fmt.Sprintf("WHOIS request sent for %s. Check your IRC client for the response.", nick)), nil
}

// RequiredPermission returns the minimum permission level needed
func (c *WhoisCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelNormal
}

// Help returns help text for this command
func (c *WhoisCommand) Help() string {
	return "!whois <nickname> - Query IRC WHOIS information for a user"
}

// CooldownDuration returns the cooldown duration for this command
func (c *WhoisCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for whois command
}

// SeenCommand implements the !seen command
type SeenCommand struct {
	db *database.DB
}

// NewSeenCommand creates a new seen command
func NewSeenCommand(db *database.DB) *SeenCommand {
	return &SeenCommand{db: db}
}

// Name returns the command name
func (c *SeenCommand) Name() string {
	return "seen"
}

// Execute runs the seen command
func (c *SeenCommand) Execute(ctx *Context) (*Response, error) {
	if len(ctx.Args) == 0 {
		return NewErrorResponse("Usage: !seen <nickname>"), nil
	}

	nick := ctx.Args[0]

	// Query database for last message from this user
	msg, err := c.db.GetLastSeen(nick)
	if err != nil {
		// Check if it's a "not found" error
		if errors.Is(err, sql.ErrNoRows) {
			return NewResponse(fmt.Sprintf("I haven't seen %s yet.", nick)), nil
		}
		return NewErrorResponse(fmt.Sprintf("Error querying database: %v", err)), nil
	}

	// Format the response
	timeSince := time.Since(msg.Timestamp)
	timeStr := formatTimeSince(timeSince)

	var location string
	if msg.Channel != "" {
		location = fmt.Sprintf("in %s", msg.Channel)
	} else {
		location = "in a private message"
	}

	// Truncate message if too long
	content := msg.Content
	if len(content) > 100 {
		content = content[:97] + "..."
	}

	message := fmt.Sprintf("%s was last seen %s ago %s, saying: %s", nick, timeStr, location, content)
	return NewResponse(message), nil
}

// RequiredPermission returns the minimum permission level needed
func (c *SeenCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelNormal
}

// Help returns help text for this command
func (c *SeenCommand) Help() string {
	return "!seen <nickname> - Show when a user was last seen and their last message"
}

// CooldownDuration returns the cooldown duration for this command
func (c *SeenCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for seen command
}

// formatTimeSince formats a duration into a human-readable "time since" string
func formatTimeSince(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	days := int(d.Hours()) / 24
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

// formatUptime formats a duration into a human-readable uptime string
func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// AuditCommand implements the !audit command
// Requirement 29.5: Add !audit command for owners
type AuditCommand struct {
	db *database.DB
}

// NewAuditCommand creates a new audit command
func NewAuditCommand(db *database.DB) *AuditCommand {
	return &AuditCommand{db: db}
}

// Name returns the command name
func (c *AuditCommand) Name() string {
	return "audit"
}

// Execute runs the audit command
func (c *AuditCommand) Execute(ctx *Context) (*Response, error) {
	// Retrieve recent audit log entries (last 10)
	entries, err := c.db.GetAuditLog(10, 0)
	if err != nil {
		return NewErrorResponse(fmt.Sprintf("Error querying audit log: %v", err)), nil
	}

	if len(entries) == 0 {
		return NewPMResponse("No audit log entries found."), nil
	}

	// Format audit log entries for display
	var message string
	message = "Recent audit log entries:\n"

	for i, entry := range entries {
		// Format timestamp
		timeStr := entry.Timestamp.Format("2006-01-02 15:04:05")

		// Format target user (nullable)
		targetStr := ""
		if entry.TargetUser != nil && *entry.TargetUser != "" {
			targetStr = fmt.Sprintf(" (target: %s)", *entry.TargetUser)
		}

		// Format details (nullable)
		detailsStr := ""
		if entry.Details != nil && *entry.Details != "" {
			detailsStr = fmt.Sprintf(" - %s", *entry.Details)
		}

		// Build entry line
		entryLine := fmt.Sprintf("[%d] %s | %s | %s: %s%s%s | Result: %s",
			i+1,
			timeStr,
			entry.ActorNick,
			entry.ActionType,
			entry.ActorHostmask,
			targetStr,
			detailsStr,
			entry.Result,
		)

		message += entryLine + "\n"
	}

	// Send as PM to avoid flooding the channel
	return NewPMResponse(message), nil
}

// RequiredPermission returns the minimum permission level needed
func (c *AuditCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelOwner
}

// Help returns help text for this command
func (c *AuditCommand) Help() string {
	return "!audit - Display recent audit log entries (owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *AuditCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for audit command
}

// MetricsCommand implements the !metrics command
// Requirement 30.4: Implement !metrics command
type MetricsCommand struct {
	db        *database.DB
	startTime time.Time
}

// NewMetricsCommand creates a new metrics command
func NewMetricsCommand(db *database.DB, startTime time.Time) *MetricsCommand {
	return &MetricsCommand{db: db, startTime: startTime}
}

// Name returns the command name
func (c *MetricsCommand) Name() string {
	return "metrics"
}

// Execute runs the metrics command
func (c *MetricsCommand) Execute(ctx *Context) (*Response, error) {
	// Get metrics stats
	stats, err := c.db.GetMetricsStats(c.startTime)
	if err != nil {
		return NewErrorResponse(fmt.Sprintf("Error querying metrics: %v", err)), nil
	}

	// Format the response
	message := c.formatMetrics(stats)
	return NewPMResponse(message), nil
}

// formatMetrics formats metrics statistics into a readable string
func (c *MetricsCommand) formatMetrics(stats *database.MetricsStats) string {
	var message string

	// Uptime
	message += "=== Bot Metrics ===\n"
	message += fmt.Sprintf("Uptime: %s\n", formatUptime(stats.Uptime))

	// Command counts
	message += "\n=== Command Usage (All Time) ===\n"
	if len(stats.CommandCounts) == 0 {
		message += "No commands executed yet.\n"
	} else {
		totalCommands := int64(0)
		for _, count := range stats.CommandCounts {
			totalCommands += count
		}
		message += fmt.Sprintf("Total Commands: %d\n", totalCommands)
		for cmd, count := range stats.CommandCounts {
			message += fmt.Sprintf("  %s: %d\n", cmd, count)
		}
	}

	// API latency
	message += "\n=== API Performance (All Time) ===\n"
	if stats.AverageAPILatency > 0 {
		message += fmt.Sprintf("Average Latency: %.2f ms\n", stats.AverageAPILatency)
	} else {
		message += "No API requests recorded.\n"
	}

	// Error counts
	message += "\n=== Errors (All Time) ===\n"
	if len(stats.ErrorCounts) == 0 {
		message += "No errors recorded.\n"
	} else {
		totalErrors := int64(0)
		for _, count := range stats.ErrorCounts {
			totalErrors += count
		}
		message += fmt.Sprintf("Total Errors: %d\n", totalErrors)
		for errType, count := range stats.ErrorCounts {
			message += fmt.Sprintf("  %s: %d\n", errType, count)
		}
	}

	// Time window stats
	message += "\n=== 24-Hour Stats ===\n"
	message += c.formatTimeWindowStats(stats.Stats24h)

	message += "\n=== 7-Day Stats ===\n"
	message += c.formatTimeWindowStats(stats.Stats7d)

	message += "\n=== 30-Day Stats ===\n"
	message += c.formatTimeWindowStats(stats.Stats30d)

	return message
}

// formatTimeWindowStats formats time window statistics
func (c *MetricsCommand) formatTimeWindowStats(stats *database.TimeWindowStats) string {
	if stats == nil {
		return "No data available.\n"
	}

	var message string
	message += fmt.Sprintf("Commands: %d (unique: %d)\n", stats.CommandCount, stats.UniqueCommands)
	if stats.AverageLatency > 0 {
		message += fmt.Sprintf("Avg API Latency: %.2f ms\n", stats.AverageLatency)
	}
	message += fmt.Sprintf("Errors: %d (types: %d)\n", stats.ErrorCount, stats.UniqueErrorTypes)

	return message
}

// RequiredPermission returns the minimum permission level needed
func (c *MetricsCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelOwner
}

// Help returns help text for this command
func (c *MetricsCommand) Help() string {
	return "!metrics - Display bot metrics and statistics (owner only)"
}

// CooldownDuration returns the cooldown duration for this command
func (c *MetricsCommand) CooldownDuration() time.Duration {
	return 0 // No cooldown for metrics command
}
