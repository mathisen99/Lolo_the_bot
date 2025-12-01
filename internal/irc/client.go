package irc

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/yourusername/lolo/internal/config"
	"github.com/yourusername/lolo/internal/output"
	"gopkg.in/irc.v4"
)

// Client wraps the irc.v4 client with additional functionality
type Client struct {
	conn      *irc.Client
	rawConn   io.ReadWriteCloser
	config    *config.Config
	logger    output.Logger
	handler   irc.Handler
	connected bool
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewClient creates a new IRC client
func NewClient(cfg *config.Config, logger output.Logger) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		config: cfg,
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
		handler: irc.HandlerFunc(func(client *irc.Client, msg *irc.Message) {
			// Default empty handler
		}),
	}
}

// Connect establishes a connection to the IRC server
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return fmt.Errorf("already connected")
	}

	// Create connection
	address := net.JoinHostPort(c.config.Server.Address, fmt.Sprintf("%d", c.config.Server.Port))

	var rawConn io.ReadWriteCloser
	var err error

	if c.config.Server.TLS {
		// TLS connection
		c.logger.Info("Connecting to %s with TLS...", address)
		tlsConfig := &tls.Config{
			ServerName: c.config.Server.Address,
		}
		rawConn, err = tls.Dial("tcp", address, tlsConfig)
		if err != nil {
			c.logger.Error("Failed to connect: %v", err)
			return fmt.Errorf("TLS connection failed: %w", err)
		}
	} else {
		// Plain connection
		c.logger.Info("Connecting to %s...", address)
		rawConn, err = net.Dial("tcp", address)
		if err != nil {
			c.logger.Error("Failed to connect: %v", err)
			return fmt.Errorf("connection failed: %w", err)
		}
	}

	c.rawConn = rawConn

	// Create IRC client configuration
	ircConfig := irc.ClientConfig{
		Nick:          c.config.Server.Nickname,
		User:          c.config.Server.Username,
		Name:          c.config.Server.Realname,
		Handler:       c.handler,
		PingFrequency: 1 * time.Minute,
		PingTimeout:   30 * time.Second,
	}

	// Create IRC client
	c.conn = irc.NewClient(rawConn, ircConfig)

	c.connected = true
	c.logger.Success("Connected to %s", address)

	return nil
}

// Disconnect closes the IRC connection
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	if c.rawConn != nil {
		_ = c.rawConn.Close()
	}

	c.connected = false
	c.cancel()
	c.logger.Info("Disconnected from IRC server")

	return nil
}

// SendMessage sends a message to a target (channel or user)
func (c *Client) SendMessage(target, message string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected || c.conn == nil {
		return fmt.Errorf("not connected")
	}

	return c.conn.WriteMessage(&irc.Message{
		Command: "PRIVMSG",
		Params:  []string{target, message},
	})
}

// JoinChannel joins an IRC channel
func (c *Client) JoinChannel(channel string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected || c.conn == nil {
		return fmt.Errorf("not connected")
	}

	c.logger.Info("Joining channel %s", channel)
	return c.conn.WriteMessage(&irc.Message{
		Command: "JOIN",
		Params:  []string{channel},
	})
}

// PartChannel leaves an IRC channel
func (c *Client) PartChannel(channel, message string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected || c.conn == nil {
		return fmt.Errorf("not connected")
	}

	params := []string{channel}
	if message != "" {
		params = append(params, message)
	}

	c.logger.Info("Leaving channel %s", channel)
	return c.conn.WriteMessage(&irc.Message{
		Command: "PART",
		Params:  params,
	})
}

// IsConnected returns whether the client is currently connected
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Write sends a raw IRC message
func (c *Client) Write(msg *irc.Message) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected || c.conn == nil {
		return fmt.Errorf("not connected")
	}

	return c.conn.WriteMessage(msg)
}

// SetHandler sets the message handler for the IRC client
// Note: This must be called before Connect() as the handler is set during client creation
func (c *Client) SetHandler(handler irc.Handler) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.handler = handler
}

// SetMessageHandler sets up a message handler function that will be called for each IRC message
// This is a convenience method that wraps the handler in irc.HandlerFunc
// Note: This must be called before Connect()
func (c *Client) SetMessageHandler(handlerFunc func(*irc.Client, *irc.Message)) {
	c.SetHandler(irc.HandlerFunc(handlerFunc))
}

// Run starts the IRC client event loop
// This will wait for a connection to be established if called before Connect()
func (c *Client) Run() error {
	// Wait for connection to be established (with timeout)
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	var conn *irc.Client
	for {
		c.mu.RLock()
		conn = c.conn
		c.mu.RUnlock()

		if conn != nil {
			break
		}

		select {
		case <-timeout:
			return fmt.Errorf("timed out waiting for connection")
		case <-ticker.C:
			// Continue waiting
		case <-c.ctx.Done():
			return fmt.Errorf("context cancelled while waiting for connection")
		}
	}

	return conn.Run()
}

// Context returns the client's context
func (c *Client) Context() context.Context {
	return c.ctx
}

// Quit sends a QUIT message to the IRC server and disconnects
func (c *Client) Quit(message string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	// Send QUIT message if we have a connection
	if c.conn != nil {
		quitMsg := &irc.Message{
			Command: "QUIT",
		}
		if message != "" {
			quitMsg.Params = []string{message}
		}

		if err := c.conn.WriteMessage(quitMsg); err != nil {
			c.logger.Error("Failed to send QUIT message: %v", err)
		}

		// Give the server a moment to process the QUIT
		time.Sleep(100 * time.Millisecond)
	}

	// Close the connection
	if c.rawConn != nil {
		_ = c.rawConn.Close()
	}

	// Always clean up state
	c.connected = false
	c.cancel()
	c.logger.Info("Sent QUIT message and disconnected")

	return nil
}
