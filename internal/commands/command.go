package commands

import (
	"time"

	"github.com/yourusername/lolo/internal/database"
)

// Command represents a bot command that can be executed
type Command interface {
	// Name returns the command name (without prefix)
	Name() string

	// Execute runs the command with the given context
	Execute(ctx *Context) (*Response, error)

	// RequiredPermission returns the minimum permission level needed to run this command
	RequiredPermission() database.PermissionLevel

	// Help returns help text for this command
	Help() string

	// CooldownDuration returns the cooldown duration for this command
	// Returns 0 if no cooldown is needed (Requirement 15.8, 15.9)
	CooldownDuration() time.Duration
}

// Context contains all information needed to execute a command
type Context struct {
	// Command name (without prefix)
	Command string

	// Arguments passed to the command
	Args []string

	// Raw message text
	RawMessage string

	// Sender information
	Nick     string
	Hostmask string

	// Channel information (empty for PMs)
	Channel string
	IsPM    bool

	// User permission level
	UserLevel database.PermissionLevel

	// Whether the user is registered in the database
	IsRegistered bool
}

// Response represents a command response
type Response struct {
	// Message to send back to the user
	Message string

	// Whether to send as a private message (overrides channel)
	SendAsPM bool

	// Target channel or user (if empty, uses the source)
	Target string

	// Whether this is an error response
	IsError bool
}

// NewContext creates a new command context
func NewContext(command string, args []string, rawMessage, nick, hostmask, channel string, isPM bool, userLevel database.PermissionLevel, isRegistered bool) *Context {
	return &Context{
		Command:      command,
		Args:         args,
		RawMessage:   rawMessage,
		Nick:         nick,
		Hostmask:     hostmask,
		Channel:      channel,
		IsPM:         isPM,
		UserLevel:    userLevel,
		IsRegistered: isRegistered,
	}
}

// NewResponse creates a new command response
func NewResponse(message string) *Response {
	return &Response{
		Message:  message,
		SendAsPM: false,
		Target:   "",
		IsError:  false,
	}
}

// NewErrorResponse creates a new error response
func NewErrorResponse(message string) *Response {
	return &Response{
		Message:  message,
		SendAsPM: false,
		Target:   "",
		IsError:  true,
	}
}

// NewPMResponse creates a new response that will be sent as a PM
func NewPMResponse(message string) *Response {
	return &Response{
		Message:  message,
		SendAsPM: true,
		Target:   "",
		IsError:  false,
	}
}
