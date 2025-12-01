package output

import (
	"fmt"
	"time"

	"github.com/fatih/color"
)

// Logger defines the interface for colored terminal output
type Logger interface {
	Info(format string, args ...interface{})
	Success(format string, args ...interface{})
	Warning(format string, args ...interface{})
	Error(format string, args ...interface{})
	ChannelMessage(channel, nick, message string)
	PrivateMessage(nick, message string)
}

// ColorLogger implements Logger with colored terminal output
type ColorLogger struct {
	infoColor    *color.Color
	successColor *color.Color
	warningColor *color.Color
	errorColor   *color.Color
	channelColor *color.Color
	pmColor      *color.Color
	nickColor    *color.Color
}

// NewColorLogger creates a new ColorLogger with default color scheme
func NewColorLogger() *ColorLogger {
	return &ColorLogger{
		infoColor:    color.New(color.FgCyan),
		successColor: color.New(color.FgGreen, color.Bold),
		warningColor: color.New(color.FgYellow, color.Bold),
		errorColor:   color.New(color.FgRed, color.Bold),
		channelColor: color.New(color.FgBlue, color.Bold),
		pmColor:      color.New(color.FgMagenta, color.Bold),
		nickColor:    color.New(color.FgGreen),
	}
}

// Info prints an informational message in cyan
func (l *ColorLogger) Info(format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, args...)
	_, _ = l.infoColor.Printf("[%s] INFO: %s\n", timestamp, message)
}

// Success prints a success message in bold green
func (l *ColorLogger) Success(format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, args...)
	_, _ = l.successColor.Printf("[%s] SUCCESS: %s\n", timestamp, message)
}

// Warning prints a warning message in bold yellow
func (l *ColorLogger) Warning(format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, args...)
	_, _ = l.warningColor.Printf("[%s] WARNING: %s\n", timestamp, message)
}

// Error prints an error message in bold red
func (l *ColorLogger) Error(format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, args...)
	_, _ = l.errorColor.Printf("[%s] ERROR: %s\n", timestamp, message)
}

// ChannelMessage prints a channel message with color-coded formatting
// Format: [HH:MM:SS] #channel <nick> message
func (l *ColorLogger) ChannelMessage(channel, nick, message string) {
	timestamp := time.Now().Format("15:04:05")
	fmt.Printf("[%s] ", timestamp)
	_, _ = l.channelColor.Printf("#%s ", channel)
	_, _ = l.nickColor.Printf("<%s> ", nick)
	fmt.Printf("%s\n", message)
}

// PrivateMessage prints a private message with distinct color formatting
// Format: [HH:MM:SS] PM from nick: message
func (l *ColorLogger) PrivateMessage(nick, message string) {
	timestamp := time.Now().Format("15:04:05")
	fmt.Printf("[%s] ", timestamp)
	_, _ = l.pmColor.Printf("PM from ")
	_, _ = l.nickColor.Printf("%s: ", nick)
	fmt.Printf("%s\n", message)
}
