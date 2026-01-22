package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yourusername/lolo/internal/circuitbreaker"
	"github.com/yourusername/lolo/internal/database"
)

// APIClientInterface defines the interface for API clients (real or mock)
type APIClientInterface interface {
	SendCommand(ctx context.Context, command string, args []string, nick, hostmask, channel string, isPM bool, timeout time.Duration) (*APIResponse, error)
	SendCommandStream(ctx context.Context, command string, args []string, nick, hostmask, channel string, isPM bool, timeout time.Duration) (<-chan *APIResponse, error)
	SendMention(ctx context.Context, message, nick, hostmask, channel, permissionLevel string, history []*database.Message, deepMode bool) (*APIResponse, error)
	SendMentionStream(ctx context.Context, message, nick, hostmask, channel, permissionLevel string, history []*database.Message, deepMode bool) (<-chan *APIResponse, error)
	CheckHealth(ctx context.Context) (*HealthResponse, error)
	GetCommands(ctx context.Context) (*CommandsResponse, error)
	WaitForInflightRequests(timeout time.Duration) bool
}

// APIClient handles communication with the Python API
type APIClient struct {
	endpoint       string
	timeout        time.Duration
	httpClient     *http.Client
	inflightReqs   sync.WaitGroup
	circuitBreaker *circuitbreaker.CircuitBreaker
}

// NewAPIClient creates a new API client with the specified endpoint and timeout
func NewAPIClient(endpoint string, timeout time.Duration) *APIClient {
	// Create HTTP client WITHOUT a global timeout.
	// Global http.Client.Timeout applies to the entire request/response cycle,
	// which is problematic for streaming responses that can legitimately take longer.
	// Instead, we use:
	// 1. Transport-level timeouts for connection establishment
	// 2. Context-based timeouts for per-request control (passed to each request)
	transport := &http.Transport{
		// Connection establishment timeouts
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second, // Time to establish TCP connection
			KeepAlive: 30 * time.Second, // Keep-alive probe interval
		}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second, // Time for TLS handshake

		// Connection pool settings for concurrent requests
		MaxIdleConns:        100,              // Max idle connections across all hosts
		MaxIdleConnsPerHost: 10,               // Max idle connections per host
		IdleConnTimeout:     90 * time.Second, // How long idle connections stay in pool

		// Response header timeout (time to receive response headers after sending request)
		// This does NOT affect streaming body reads
		ResponseHeaderTimeout: 30 * time.Second,

		// Disable compression for streaming to avoid buffering issues
		DisableCompression: false,
	}

	client := &APIClient{
		endpoint: endpoint,
		timeout:  timeout,
		httpClient: &http.Client{
			// NO global Timeout - this is critical for streaming!
			// Timeout: timeout would kill long-running streams
			Transport: transport,
		},
	}

	// Create circuit breaker with health check function
	client.circuitBreaker = circuitbreaker.New(circuitbreaker.Config{
		Threshold: 5,                // Open after 5 consecutive failures
		Timeout:   30 * time.Second, // Wait 30 seconds before retry
		HealthCheckFn: func(ctx context.Context) error {
			_, err := client.CheckHealth(ctx)
			return err
		},
	})

	return client
}

// CommandRequest represents a command request to the Python API
type CommandRequest struct {
	RequestID string   `json:"request_id"`
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	Nick      string   `json:"nick"`
	Hostmask  string   `json:"hostmask"`
	Channel   string   `json:"channel"`
	IsPM      bool     `json:"is_pm"`
}

// HistoryMessage represents a message in conversation history
type HistoryMessage struct {
	Timestamp string `json:"timestamp"`
	Nick      string `json:"nick"`
	Content   string `json:"content"`
}

// MentionRequest represents a mention request to the Python API
type MentionRequest struct {
	RequestID       string           `json:"request_id"`
	Message         string           `json:"message"`
	Nick            string           `json:"nick"`
	Hostmask        string           `json:"hostmask"`
	Channel         string           `json:"channel"`
	PermissionLevel string           `json:"permission_level"`
	History         []HistoryMessage `json:"history,omitempty"`
	DeepMode        bool             `json:"deep_mode,omitempty"`
}

// APIResponse represents a response from the Python API
type APIResponse struct {
	RequestID     string `json:"request_id"`
	Status        string `json:"status"`
	Message       string `json:"message"`
	RequiredLevel string `json:"required_level,omitempty"`
	Streaming     bool   `json:"streaming,omitempty"`
}

// HealthResponse represents a response from the /health endpoint
type HealthResponse struct {
	Status  string  `json:"status"`
	Uptime  float64 `json:"uptime"` // Changed to float64 to match Python API
	Version string  `json:"version"`
}

// CommandMetadata represents metadata for a command from the /commands endpoint
type CommandMetadata struct {
	Name               string           `json:"name"`
	HelpText           string           `json:"help_text"`
	RequiredPermission string           `json:"required_permission"`
	Arguments          []ArgumentSchema `json:"arguments"`
	Timeout            int              `json:"timeout"`
	Cooldown           int              `json:"cooldown"`
	Streaming          bool             `json:"streaming"`
}

// ArgumentSchema represents the schema for a command argument
type ArgumentSchema struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

// CommandsResponse represents a response from the /commands endpoint
type CommandsResponse struct {
	Commands []CommandMetadata `json:"commands"`
}

// SendCommand sends a command request to the Python API
// If timeout is 0, uses the default client timeout
func (c *APIClient) SendCommand(ctx context.Context, command string, args []string, nick, hostmask, channel string, isPM bool, timeout time.Duration) (*APIResponse, error) {
	requestID := uuid.New().String()

	req := CommandRequest{
		RequestID: requestID,
		Command:   command,
		Args:      args,
		Nick:      nick,
		Hostmask:  hostmask,
		Channel:   channel,
		IsPM:      isPM,
	}

	var response *APIResponse
	var err error

	// Use circuit breaker to protect API calls
	cbErr := c.circuitBreaker.Call(ctx, func() error {
		response, err = c.sendRequest(ctx, "/command", req, requestID, timeout)
		return err
	})

	if cbErr != nil {
		// Circuit breaker blocked the call
		return nil, fmt.Errorf("circuit breaker: %w", cbErr)
	}

	return response, err
}

// SendCommandStream sends a streaming command request to the Python API
// Returns a channel that receives response chunks as they arrive
// If timeout is 0, uses the default client timeout
func (c *APIClient) SendCommandStream(ctx context.Context, command string, args []string, nick, hostmask, channel string, isPM bool, timeout time.Duration) (<-chan *APIResponse, error) {
	requestID := uuid.New().String()

	req := CommandRequest{
		RequestID: requestID,
		Command:   command,
		Args:      args,
		Nick:      nick,
		Hostmask:  hostmask,
		Channel:   channel,
		IsPM:      isPM,
	}

	// Create channel for responses
	responseChan := make(chan *APIResponse, 10) // Buffered channel to avoid blocking

	// Use circuit breaker to protect API calls
	cbErr := c.circuitBreaker.Call(ctx, func() error {
		go c.streamRequest(ctx, "/command/stream", req, requestID, responseChan, timeout)
		return nil
	})

	if cbErr != nil {
		// Circuit breaker blocked the call
		close(responseChan)
		return nil, fmt.Errorf("circuit breaker: %w", cbErr)
	}

	return responseChan, nil
}

// SendMention sends a mention request to the Python API with conversation history
func (c *APIClient) SendMention(ctx context.Context, message, nick, hostmask, channel, permissionLevel string, history []*database.Message, deepMode bool) (*APIResponse, error) {
	requestID := uuid.New().String()

	// Convert database messages to API format
	historyMessages := make([]HistoryMessage, 0, len(history))
	for _, msg := range history {
		historyMessages = append(historyMessages, HistoryMessage{
			Timestamp: msg.Timestamp.Format("2006-01-02 15:04:05"),
			Nick:      msg.Nick,
			Content:   msg.Content,
		})
	}

	req := MentionRequest{
		RequestID:       requestID,
		Message:         message,
		Nick:            nick,
		Hostmask:        hostmask,
		Channel:         channel,
		PermissionLevel: permissionLevel,
		History:         historyMessages,
		DeepMode:        deepMode,
	}

	var response *APIResponse
	var err error

	// Use circuit breaker to protect API calls
	cbErr := c.circuitBreaker.Call(ctx, func() error {
		response, err = c.sendRequest(ctx, "/mention", req, requestID, 0) // Use default timeout for mentions
		return err
	})

	if cbErr != nil {
		// Circuit breaker blocked the call
		return nil, fmt.Errorf("circuit breaker: %w", cbErr)
	}

	return response, err
}

// SendMentionStream sends a streaming mention request to the Python API with conversation history
// Returns a channel that receives response chunks as they arrive
func (c *APIClient) SendMentionStream(ctx context.Context, message, nick, hostmask, channel, permissionLevel string, history []*database.Message, deepMode bool) (<-chan *APIResponse, error) {
	requestID := uuid.New().String()

	// Convert database messages to API format
	historyMessages := make([]HistoryMessage, 0, len(history))
	for _, msg := range history {
		historyMessages = append(historyMessages, HistoryMessage{
			Timestamp: msg.Timestamp.Format("2006-01-02 15:04:05"),
			Nick:      msg.Nick,
			Content:   msg.Content,
		})
	}

	req := MentionRequest{
		RequestID:       requestID,
		Message:         message,
		Nick:            nick,
		Hostmask:        hostmask,
		Channel:         channel,
		PermissionLevel: permissionLevel,
		History:         historyMessages,
		DeepMode:        deepMode,
	}

	// Create channel for responses
	responseChan := make(chan *APIResponse, 10) // Buffered channel to avoid blocking

	// Use circuit breaker to protect API calls
	cbErr := c.circuitBreaker.Call(ctx, func() error {
		go c.streamRequest(ctx, "/mention/stream", req, requestID, responseChan, 0)
		return nil
	})

	if cbErr != nil {
		// Circuit breaker blocked the call
		close(responseChan)
		return nil, fmt.Errorf("circuit breaker: %w", cbErr)
	}

	return responseChan, nil
}

// CheckHealth checks if the Python API is available
func (c *APIClient) CheckHealth(ctx context.Context) (*HealthResponse, error) {
	url := c.endpoint + "/health"

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("health check request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read health check response: %w", err)
	}

	var healthResp HealthResponse
	if err := json.Unmarshal(body, &healthResp); err != nil {
		return nil, fmt.Errorf("failed to parse health check response: %w", err)
	}

	return &healthResp, nil
}

// GetCommands retrieves command metadata from the Python API
func (c *APIClient) GetCommands(ctx context.Context) (*CommandsResponse, error) {
	url := c.endpoint + "/commands"

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create commands request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("commands request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("commands endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read commands response: %w", err)
	}

	var commandsResp CommandsResponse
	if err := json.Unmarshal(body, &commandsResp); err != nil {
		return nil, fmt.Errorf("failed to parse commands response: %w", err)
	}

	return &commandsResp, nil
}

// sendRequest is a helper method to send a request to the Python API with retry logic
// If timeout is 0, uses the default client timeout
func (c *APIClient) sendRequest(ctx context.Context, endpoint string, payload interface{}, requestID string, timeout time.Duration) (*APIResponse, error) {
	// Track in-flight request
	c.inflightReqs.Add(1)
	defer c.inflightReqs.Done()

	const maxRetries = 3
	backoffDelays := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}

	var lastErr error

	// Apply custom timeout if specified
	if timeout > 0 {
		fmt.Printf("[API Client] Sending request [%s] to %s (timeout: %v)\n", requestID, endpoint, timeout)
		// Create a new context with the custom timeout
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	} else {
		// Log the initial request with default timeout
		fmt.Printf("[API Client] Sending request [%s] to %s (timeout: %v)\n", requestID, endpoint, c.timeout)
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// If this is a retry, apply backoff delay
		if attempt > 0 {
			delay := backoffDelays[attempt-1]
			fmt.Printf("[API Client] Retry attempt %d/%d for request [%s] after %v\n", attempt, maxRetries, requestID, delay)

			select {
			case <-time.After(delay):
				// Continue with retry
			case <-ctx.Done():
				return nil, fmt.Errorf("[%s] context cancelled during retry backoff: %w", requestID, ctx.Err())
			}
		}

		// Attempt the request
		response, err := c.doRequest(ctx, endpoint, payload, requestID)
		if err == nil {
			// Success - return the response
			if attempt > 0 {
				fmt.Printf("[API Client] Request [%s] succeeded on retry attempt %d\n", requestID, attempt)
			} else {
				fmt.Printf("[API Client] Request [%s] completed successfully\n", requestID)
			}
			return response, nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			// Non-retryable error - fail immediately
			fmt.Printf("[API Client] Request [%s] failed with non-retryable error: %v\n", requestID, err)
			return nil, fmt.Errorf("[%s] %w", requestID, err)
		}

		// Check if we've exhausted retries
		if attempt >= maxRetries {
			fmt.Printf("[API Client] Request [%s] failed after %d retries: %v\n", requestID, maxRetries, err)
			break
		}

		// Log the retry
		fmt.Printf("[API Client] Request [%s] failed (attempt %d/%d): %v - will retry\n", requestID, attempt+1, maxRetries+1, err)
	}

	return nil, fmt.Errorf("[%s] request failed after %d retries: %w", requestID, maxRetries, lastErr)
}

// doRequest performs a single HTTP request attempt without retry logic
func (c *APIClient) doRequest(ctx context.Context, endpoint string, payload interface{}, requestID string) (*APIResponse, error) {
	url := c.endpoint + endpoint

	// Marshal the request payload
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request with context
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Request-ID", requestID)

	// Send the request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// For 4xx errors, try to parse FastAPI validation error format first
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		var fastAPIError struct {
			Detail interface{} `json:"detail"`
		}
		if json.Unmarshal(body, &fastAPIError) == nil {
			// Try to extract the message from the detail field
			if detailMap, ok := fastAPIError.Detail.(map[string]interface{}); ok {
				if msg, ok := detailMap["message"].(string); ok {
					// Extract request_id if available
					reqID := requestID
					if id, ok := detailMap["request_id"].(string); ok {
						reqID = id
					}
					// Return a clean error response (not an error, so it won't retry)
					return &APIResponse{
						RequestID: reqID,
						Status:    "error",
						Message:   msg,
					}, nil
				}
			}
		}
	}

	// Parse the response (works for both success and error responses)
	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w (body: %s)", err, string(body))
	}

	// Check for non-2xx status codes after parsing
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// If we have a parsed response with a message, use it
		if apiResp.Message != "" {
			return &apiResp, nil
		}
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Validate response structure - ensure all required fields are present
	if err := validateAPIResponse(&apiResp); err != nil {
		return nil, fmt.Errorf("malformed response: %w (body: %s)", err, string(body))
	}

	return &apiResp, nil
}

// isRetryableError determines if an error is transient and should be retried
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Network errors are retryable
	if contains(errStr, "connection refused") ||
		contains(errStr, "connection reset") ||
		contains(errStr, "broken pipe") ||
		contains(errStr, "no such host") ||
		contains(errStr, "network is unreachable") ||
		contains(errStr, "i/o timeout") ||
		contains(errStr, "request failed") {
		return true
	}

	// 5xx server errors are retryable (503 Service Unavailable, 500 Internal Server Error, etc.)
	if contains(errStr, "status 5") {
		return true
	}

	// 429 Too Many Requests is retryable
	if contains(errStr, "status 429") {
		return true
	}

	// 4xx client errors (except 429) are NOT retryable
	// 2xx success codes are NOT retryable (shouldn't reach here anyway)
	return false
}

// contains checks if a string contains a substring (case-sensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// SetTimeout updates the client timeout
// Note: This only affects the stored timeout value used for context-based timeouts.
// The HTTP client no longer uses a global timeout (for streaming compatibility).
func (c *APIClient) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
	// Note: We no longer set httpClient.Timeout as it interferes with streaming
}

// GetEndpoint returns the API endpoint
func (c *APIClient) GetEndpoint() string {
	return c.endpoint
}

// WaitForInflightRequests waits for all in-flight API requests to complete
// Returns true if all requests completed, false if timeout occurred
func (c *APIClient) WaitForInflightRequests(timeout time.Duration) bool {
	done := make(chan struct{})

	go func() {
		c.inflightReqs.Wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// NullResponseMarker is a special marker that indicates the bot should not respond
// Used by the null_response tool when users explicitly request silence
const NullResponseMarker = "<<NULL_RESPONSE>>"

// validateAPIResponse validates that an API response contains all required fields
// and that the values are valid according to the API contract
func validateAPIResponse(resp *APIResponse) error {
	// Check required fields
	if resp.Status == "" {
		return fmt.Errorf("missing required field: status")
	}

	// Allow empty message if it's a null response (user requested silence)
	if resp.Message == "" && resp.Status != "null" {
		return fmt.Errorf("missing required field: message")
	}

	// Validate status field values
	if resp.Status != "success" && resp.Status != "error" && resp.Status != "null" && resp.Status != "processing" {
		return fmt.Errorf("invalid status value: %q (must be 'success', 'error', 'null', or 'processing')", resp.Status)
	}

	// Validate optional required_level field if present
	if resp.RequiredLevel != "" {
		validLevels := map[string]bool{
			"owner":  true,
			"admin":  true,
			"normal": true,
			"any":    true,
		}
		if !validLevels[resp.RequiredLevel] {
			return fmt.Errorf("invalid required_level value: %q (must be 'owner', 'admin', 'normal', or 'any')", resp.RequiredLevel)
		}
	}

	return nil
}

// GetCircuitBreakerState returns the current state of the circuit breaker as a string
func (c *APIClient) GetCircuitBreakerState() string {
	return c.circuitBreaker.GetState().String()
}

// GetCircuitBreakerStateEnum returns the current state of the circuit breaker
func (c *APIClient) GetCircuitBreakerStateEnum() circuitbreaker.State {
	return c.circuitBreaker.GetState()
}

// GetCircuitBreakerStats returns statistics about the circuit breaker
func (c *APIClient) GetCircuitBreakerStats() circuitbreaker.Stats {
	return c.circuitBreaker.GetStats()
}

// TryHealthCheck attempts to perform a health check to test if the circuit can be closed
func (c *APIClient) TryHealthCheck(ctx context.Context) error {
	return c.circuitBreaker.TryHealthCheck(ctx)
}

// streamRequest handles streaming responses from the Python API
// Reads newline-delimited JSON (NDJSON) and sends each chunk to the response channel
// If timeout is 0, uses the default client timeout
func (c *APIClient) streamRequest(ctx context.Context, endpoint string, payload interface{}, requestID string, responseChan chan<- *APIResponse, timeout time.Duration) {
	// Track in-flight request
	c.inflightReqs.Add(1)
	defer c.inflightReqs.Done()
	defer close(responseChan)

	const maxRetries = 3
	backoffDelays := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}

	var lastErr error

	// Apply custom timeout if specified
	if timeout > 0 {
		fmt.Printf("[API Client] Starting streaming request [%s] to %s (timeout: %v)\n", requestID, endpoint, timeout)
		// Create a new context with the custom timeout
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	} else {
		// Log the initial request with default timeout
		fmt.Printf("[API Client] Starting streaming request [%s] to %s (timeout: %v)\n", requestID, endpoint, c.timeout)
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// If this is a retry, apply backoff delay
		if attempt > 0 {
			delay := backoffDelays[attempt-1]
			fmt.Printf("[API Client] Retry attempt %d/%d for streaming request [%s] after %v\n", attempt, maxRetries, requestID, delay)

			select {
			case <-time.After(delay):
				// Continue with retry
			case <-ctx.Done():
				responseChan <- &APIResponse{
					RequestID: requestID,
					Status:    "error",
					Message:   fmt.Sprintf("context cancelled during retry backoff: %v", ctx.Err()),
					Streaming: false,
				}
				return
			}
		}

		// Attempt the streaming request
		err := c.doStreamRequest(ctx, endpoint, payload, requestID, responseChan)
		if err == nil {
			// Success
			if attempt > 0 {
				fmt.Printf("[API Client] Streaming request [%s] succeeded on retry attempt %d\n", requestID, attempt)
			} else {
				fmt.Printf("[API Client] Streaming request [%s] completed successfully\n", requestID)
			}
			return
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			// Non-retryable error - fail immediately
			fmt.Printf("[API Client] Streaming request [%s] failed with non-retryable error: %v\n", requestID, err)
			responseChan <- &APIResponse{
				RequestID: requestID,
				Status:    "error",
				Message:   fmt.Sprintf("streaming request failed: %v", err),
				Streaming: false,
			}
			return
		}

		// Check if we've exhausted retries
		if attempt >= maxRetries {
			fmt.Printf("[API Client] Streaming request [%s] failed after %d retries: %v\n", requestID, maxRetries, err)
			break
		}

		// Log the retry
		fmt.Printf("[API Client] Streaming request [%s] failed (attempt %d/%d): %v - will retry\n", requestID, attempt+1, maxRetries+1, err)
	}

	// All retries exhausted
	responseChan <- &APIResponse{
		RequestID: requestID,
		Status:    "error",
		Message:   fmt.Sprintf("streaming request failed after %d retries: %v", maxRetries, lastErr),
		Streaming: false,
	}
}

// doStreamRequest performs a single streaming HTTP request
func (c *APIClient) doStreamRequest(ctx context.Context, endpoint string, payload interface{}, requestID string, responseChan chan<- *APIResponse) error {
	url := c.endpoint + endpoint

	// Marshal the request payload
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request with context
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Request-ID", requestID)

	// Send the request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Check for non-2xx status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read streaming response (newline-delimited JSON)
	scanner := bufio.NewScanner(resp.Body)
	chunkCount := 0

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue // Skip empty lines
		}

		// Parse JSON chunk
		var apiResp APIResponse
		if err := json.Unmarshal(line, &apiResp); err != nil {
			fmt.Printf("[API Client] Failed to parse streaming chunk [%s]: %v\n", requestID, err)
			responseChan <- &APIResponse{
				RequestID: requestID,
				Status:    "error",
				Message:   fmt.Sprintf("failed to parse response chunk: %v", err),
				Streaming: false,
			}
			return fmt.Errorf("failed to parse response chunk: %w", err)
		}

		// Validate response structure
		if err := validateAPIResponse(&apiResp); err != nil {
			fmt.Printf("[API Client] Invalid streaming response [%s]: %v\n", requestID, err)
			responseChan <- &APIResponse{
				RequestID: requestID,
				Status:    "error",
				Message:   fmt.Sprintf("malformed response chunk: %v", err),
				Streaming: false,
			}
			return fmt.Errorf("malformed response chunk: %w", err)
		}

		// Send chunk to response channel
		responseChan <- &apiResp
		chunkCount++

		// Check if this is the last chunk (streaming=false)
		if !apiResp.Streaming {
			fmt.Printf("[API Client] Streaming request [%s] completed with %d chunks\n", requestID, chunkCount)
			return nil
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		fmt.Printf("[API Client] Error reading streaming response [%s]: %v\n", requestID, err)
		return fmt.Errorf("error reading streaming response: %w", err)
	}

	// If we get here without seeing streaming=false, it's an error
	fmt.Printf("[API Client] Streaming request [%s] ended without final chunk\n", requestID)
	return fmt.Errorf("streaming response ended without final chunk")
}
