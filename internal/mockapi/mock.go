package mockapi

import (
	"context"
	"fmt"
	"time"

	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/handler"
)

// MockAPIClient provides mock responses for testing without a real Python API
type MockAPIClient struct {
	// Map of command names to mock responses
	responses map[string]string
	// Simulate latency
	latency time.Duration
	// Track if health check should fail
	healthCheckFail bool
}

// New creates a new mock API client
func New() *MockAPIClient {
	return &MockAPIClient{
		responses: map[string]string{
			"test": "test succeeded",
			"ping": "pong",
		},
		latency: 0,
	}
}

// SendCommand returns a mock response for the given command
func (m *MockAPIClient) SendCommand(ctx context.Context, command string, args []string, nick, hostmask, channel string, isPM bool, timeout time.Duration) (*handler.APIResponse, error) {
	// Simulate latency if configured
	if m.latency > 0 {
		select {
		case <-time.After(m.latency):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Generate mock response
	response := &handler.APIResponse{
		RequestID: fmt.Sprintf("mock-%d", time.Now().UnixNano()),
		Status:    "success",
	}

	// Check if we have a predefined response for this command
	if msg, ok := m.responses[command]; ok {
		response.Message = msg
		return response, nil
	}

	// Default response for unknown commands
	response.Status = "error"
	response.Message = fmt.Sprintf("Unknown command: %s", command)
	return response, nil
}

// SendCommandStream returns a mock streaming response
func (m *MockAPIClient) SendCommandStream(ctx context.Context, command string, args []string, nick, hostmask, channel string, isPM bool, timeout time.Duration) (<-chan *handler.APIResponse, error) {
	responseChan := make(chan *handler.APIResponse, 1)

	go func() {
		defer close(responseChan)

		// Simulate latency if configured
		if m.latency > 0 {
			select {
			case <-time.After(m.latency):
			case <-ctx.Done():
				return
			}
		}

		// Generate mock response
		response := &handler.APIResponse{
			RequestID: fmt.Sprintf("mock-%d", time.Now().UnixNano()),
			Status:    "success",
		}

		// Check if we have a predefined response for this command
		if msg, ok := m.responses[command]; ok {
			response.Message = msg
		} else {
			response.Status = "error"
			response.Message = fmt.Sprintf("Unknown command: %s", command)
		}

		responseChan <- response
	}()

	return responseChan, nil
}

// SendMention returns a mock response for mentions
func (m *MockAPIClient) SendMention(ctx context.Context, message, nick, hostmask, channel, permissionLevel string, history []*database.Message) (*handler.APIResponse, error) {
	// Simulate latency if configured
	if m.latency > 0 {
		select {
		case <-time.After(m.latency):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Build response with context awareness if history is provided
	responseMsg := fmt.Sprintf("Thanks for mentioning me, %s!", nick)
	if len(history) > 0 {
		responseMsg = fmt.Sprintf("Thanks for mentioning me, %s! I can see the last %d messages for context.", nick, len(history))
	}

	response := &handler.APIResponse{
		RequestID: fmt.Sprintf("mock-%d", time.Now().UnixNano()),
		Status:    "success",
		Message:   responseMsg,
	}

	return response, nil
}

// CheckHealth returns a mock health response
func (m *MockAPIClient) CheckHealth(ctx context.Context) (*handler.HealthResponse, error) {
	if m.healthCheckFail {
		return nil, fmt.Errorf("mock health check failed")
	}

	return &handler.HealthResponse{
		Status:  "ok",
		Uptime:  time.Since(time.Now().Add(-1 * time.Hour)).Seconds(),
		Version: "mock-1.0.0",
	}, nil
}

// GetCommands returns mock command metadata
func (m *MockAPIClient) GetCommands(ctx context.Context) (*handler.CommandsResponse, error) {
	return &handler.CommandsResponse{
		Commands: []handler.CommandMetadata{
			{
				Name:               "test",
				HelpText:           "Test command that returns 'test succeeded'",
				RequiredPermission: "any",
				Arguments:          []handler.ArgumentSchema{},
				Timeout:            240,
				Cooldown:           3,
				Streaming:          false,
			},
			{
				Name:               "ping",
				HelpText:           "Ping command that returns 'pong'",
				RequiredPermission: "any",
				Arguments:          []handler.ArgumentSchema{},
				Timeout:            240,
				Cooldown:           3,
				Streaming:          false,
			},
		},
	}, nil
}

// WaitForInflightRequests is a no-op for mock client
func (m *MockAPIClient) WaitForInflightRequests(timeout time.Duration) bool {
	return true
}

// SetLatency sets the simulated latency for mock responses
func (m *MockAPIClient) SetLatency(latency time.Duration) {
	m.latency = latency
}

// SetHealthCheckFail sets whether health checks should fail
func (m *MockAPIClient) SetHealthCheckFail(fail bool) {
	m.healthCheckFail = fail
}

// SetResponse sets a custom response for a command
func (m *MockAPIClient) SetResponse(command, response string) {
	m.responses[command] = response
}

// GetResponse gets the current response for a command
func (m *MockAPIClient) GetResponse(command string) (string, bool) {
	resp, ok := m.responses[command]
	return resp, ok
}
