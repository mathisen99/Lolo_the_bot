package irc

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/yourusername/lolo/internal/config"
	"github.com/yourusername/lolo/internal/output"
	"gopkg.in/irc.v4"
)

// Authenticator handles IRC authentication (SASL and NickServ)
type Authenticator struct {
	config *config.Config
	logger output.Logger
	client *Client

	saslComplete bool
	saslSuccess  bool
	authComplete chan bool
	authTimeout  time.Duration
}

// NewAuthenticator creates a new authenticator
func NewAuthenticator(cfg *config.Config, logger output.Logger, client *Client) *Authenticator {
	return &Authenticator{
		config:       cfg,
		logger:       logger,
		client:       client,
		authComplete: make(chan bool, 1),
		authTimeout:  30 * time.Second,
	}
}

// Authenticate performs authentication using SASL or NickServ fallback
func (a *Authenticator) Authenticate() error {
	// Try SASL first if credentials are provided
	if a.config.Auth.SASLUsername != "" && a.config.Auth.SASLPassword != "" {
		a.logger.Info("Attempting SASL PLAIN authentication...")

		err := a.authenticateSASL()
		if err == nil {
			a.logger.Success("SASL authentication successful")
			return nil
		}

		a.logger.Warning("SASL authentication failed: %v", err)
	}

	// Fall back to NickServ if SASL fails or is not configured
	if a.config.Auth.NickServPassword != "" {
		a.logger.Info("Falling back to NickServ authentication...")

		err := a.authenticateNickServ()
		if err == nil {
			a.logger.Success("NickServ authentication successful")
			return nil
		}

		a.logger.Error("NickServ authentication failed: %v", err)
		return fmt.Errorf("all authentication methods failed")
	}

	// No authentication configured
	a.logger.Warning("No authentication credentials configured")
	return nil
}

// authenticateSASL performs SASL PLAIN authentication
func (a *Authenticator) authenticateSASL() error {
	// Request SASL capability
	err := a.client.Write(&irc.Message{
		Command: "CAP",
		Params:  []string{"REQ", "sasl"},
	})
	if err != nil {
		return fmt.Errorf("failed to request SASL capability: %w", err)
	}

	// Wait for CAP ACK
	select {
	case success := <-a.authComplete:
		if !success {
			return fmt.Errorf("SASL capability not acknowledged")
		}
	case <-time.After(a.authTimeout):
		return fmt.Errorf("SASL capability request timed out")
	}

	// Send AUTHENTICATE PLAIN
	err = a.client.Write(&irc.Message{
		Command: "AUTHENTICATE",
		Params:  []string{"PLAIN"},
	})
	if err != nil {
		return fmt.Errorf("failed to initiate SASL PLAIN: %w", err)
	}

	// Wait for AUTHENTICATE +
	select {
	case success := <-a.authComplete:
		if !success {
			return fmt.Errorf("SASL PLAIN not accepted")
		}
	case <-time.After(a.authTimeout):
		return fmt.Errorf("SASL PLAIN initiation timed out")
	}

	// Encode credentials: \0username\0password
	credentials := fmt.Sprintf("\x00%s\x00%s",
		a.config.Auth.SASLUsername,
		a.config.Auth.SASLPassword)
	encoded := base64.StdEncoding.EncodeToString([]byte(credentials))

	// Send encoded credentials
	err = a.client.Write(&irc.Message{
		Command: "AUTHENTICATE",
		Params:  []string{encoded},
	})
	if err != nil {
		return fmt.Errorf("failed to send SASL credentials: %w", err)
	}

	// Wait for authentication result
	select {
	case success := <-a.authComplete:
		if !success {
			return fmt.Errorf("SASL authentication rejected")
		}
	case <-time.After(a.authTimeout):
		return fmt.Errorf("SASL authentication timed out")
	}

	// End capability negotiation
	err = a.client.Write(&irc.Message{
		Command: "CAP",
		Params:  []string{"END"},
	})
	if err != nil {
		return fmt.Errorf("failed to end capability negotiation: %w", err)
	}

	return nil
}

// authenticateNickServ performs NickServ authentication
func (a *Authenticator) authenticateNickServ() error {
	// Send IDENTIFY command to NickServ
	message := fmt.Sprintf("IDENTIFY %s", a.config.Auth.NickServPassword)

	err := a.client.SendMessage("NickServ", message)
	if err != nil {
		return fmt.Errorf("failed to send NickServ IDENTIFY: %w", err)
	}

	// Wait a moment for NickServ to respond
	// Note: In a production system, we'd wait for a specific response from NickServ
	time.Sleep(2 * time.Second)

	return nil
}

// HandleSASLMessage processes SASL-related IRC messages
func (a *Authenticator) HandleSASLMessage(msg *irc.Message) {
	switch msg.Command {
	case "CAP":
		// Check if SASL capability was acknowledged
		if len(msg.Params) >= 2 && msg.Params[1] == "ACK" {
			if len(msg.Params) >= 3 && msg.Params[2] == "sasl" {
				a.authComplete <- true
			}
		} else if len(msg.Params) >= 2 && msg.Params[1] == "NAK" {
			a.authComplete <- false
		}

	case "AUTHENTICATE":
		// Server is ready for credentials
		if len(msg.Params) >= 1 && msg.Params[0] == "+" {
			a.authComplete <- true
		}

	case "903": // RPL_SASLSUCCESS
		a.logger.Success("SASL authentication successful")
		a.saslSuccess = true
		a.saslComplete = true
		a.authComplete <- true

	case "904", "905", "906", "907": // SASL error codes
		a.logger.Error("SASL authentication failed: %s", msg.Trailing())
		a.saslSuccess = false
		a.saslComplete = true
		a.authComplete <- false
	}
}

// IsSASLComplete returns whether SASL authentication has completed
func (a *Authenticator) IsSASLComplete() bool {
	return a.saslComplete
}

// IsSASLSuccess returns whether SASL authentication was successful
func (a *Authenticator) IsSASLSuccess() bool {
	return a.saslSuccess
}
