package codexreset

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// Snapshot is the subset of Codex account state used by the watcher.
type Snapshot struct {
	RateLimits        map[string]RateLimitSnapshot
	ResetCredits      *ResetCreditsSummary
	WorkspaceMessages *WorkspaceMessagesSnapshot
}

type ResetCreditsSummary struct {
	AvailableCount int64 `json:"availableCount"`
}

type RateLimitSnapshot struct {
	LimitID   string           `json:"limitId"`
	Primary   *RateLimitWindow `json:"primary"`
	Secondary *RateLimitWindow `json:"secondary"`
}

type RateLimitWindow struct {
	UsedPercent        int    `json:"usedPercent"`
	WindowDurationMins *int64 `json:"windowDurationMins"`
	ResetsAt           *int64 `json:"resetsAt"`
}

type WorkspaceMessagesSnapshot struct {
	FeatureEnabled bool               `json:"featureEnabled"`
	Messages       []WorkspaceMessage `json:"messages"`
}

type WorkspaceMessage struct {
	ID         string `json:"messageId"`
	Type       string `json:"messageType"`
	Body       string `json:"messageBody"`
	CreatedAt  *int64 `json:"createdAt"`
	ArchivedAt *int64 `json:"archivedAt"`
}

type snapshotSource interface {
	Read(context.Context) (Snapshot, error)
}

// AppServerSource reads authenticated account state through the installed
// Codex app-server JSONL protocol. A short-lived process per poll keeps failure
// recovery simple and avoids coupling the IRC bot to an experimental daemon.
type AppServerSource struct {
	CodexPath string
}

type rpcEnvelope struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rateLimitsResult struct {
	RateLimits          *RateLimitSnapshot           `json:"rateLimits"`
	RateLimitsByLimitID map[string]RateLimitSnapshot `json:"rateLimitsByLimitId"`
	ResetCredits        *ResetCreditsSummary         `json:"rateLimitResetCredits"`
}

func (s *AppServerSource) Read(ctx context.Context) (Snapshot, error) {
	command := strings.TrimSpace(s.CodexPath)
	if command == "" {
		command = "codex"
	}

	cmd := exec.CommandContext(ctx, command, "app-server", "--listen", "stdio://")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Snapshot{}, fmt.Errorf("open Codex app-server stdout: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Snapshot{}, fmt.Errorf("open Codex app-server stdin: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return Snapshot{}, fmt.Errorf("start Codex app-server: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	encoder := json.NewEncoder(stdin)
	if err := encoder.Encode(map[string]any{
		"method": "initialize",
		"id":     1,
		"params": map[string]any{
			"clientInfo": map[string]string{
				"name":    "lolo_irc_bot",
				"title":   "Lolo IRC Bot",
				"version": "1.0.0",
			},
		},
	}); err != nil {
		return Snapshot{}, fmt.Errorf("initialize Codex app-server: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	if _, err := readRPCResponse(ctx, scanner, 1); err != nil {
		return Snapshot{}, withAppServerStderr(err, stderr.String())
	}

	if err := encoder.Encode(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		return Snapshot{}, fmt.Errorf("acknowledge Codex app-server initialization: %w", err)
	}
	if err := encoder.Encode(map[string]any{"method": "account/rateLimits/read", "id": 2}); err != nil {
		return Snapshot{}, fmt.Errorf("request Codex rate limits: %w", err)
	}
	if err := encoder.Encode(map[string]any{"method": "account/workspaceMessages/read", "id": 3}); err != nil {
		return Snapshot{}, fmt.Errorf("request Codex workspace messages: %w", err)
	}

	var snapshot Snapshot
	gotRateLimits := false
	gotWorkspaceMessages := false
	for !gotRateLimits || !gotWorkspaceMessages {
		envelope, err := readNextRPCEnvelope(ctx, scanner)
		if err != nil {
			return Snapshot{}, withAppServerStderr(err, stderr.String())
		}
		switch rpcID(envelope.ID) {
		case 2:
			gotRateLimits = true
			if envelope.Error != nil {
				return Snapshot{}, fmt.Errorf("Codex rate-limit read failed (%d): %s", envelope.Error.Code, envelope.Error.Message)
			}
			var result rateLimitsResult
			if err := json.Unmarshal(envelope.Result, &result); err != nil {
				return Snapshot{}, fmt.Errorf("decode Codex rate-limit response: %w", err)
			}
			snapshot.RateLimits = normalizeRateLimits(result)
			snapshot.ResetCredits = result.ResetCredits
		case 3:
			gotWorkspaceMessages = true
			// Workspace messages are optional. An unavailable route must not hide
			// reset detection from the rate-limit window snapshots.
			if envelope.Error == nil {
				var result WorkspaceMessagesSnapshot
				if err := json.Unmarshal(envelope.Result, &result); err != nil {
					return Snapshot{}, fmt.Errorf("decode Codex workspace-message response: %w", err)
				}
				snapshot.WorkspaceMessages = &result
			}
		}
	}

	return snapshot, nil
}

func normalizeRateLimits(result rateLimitsResult) map[string]RateLimitSnapshot {
	if len(result.RateLimitsByLimitID) > 0 {
		normalized := make(map[string]RateLimitSnapshot, len(result.RateLimitsByLimitID))
		for key, snapshot := range result.RateLimitsByLimitID {
			if strings.TrimSpace(snapshot.LimitID) == "" {
				snapshot.LimitID = key
			}
			normalized[key] = snapshot
		}
		return normalized
	}
	if result.RateLimits == nil {
		return nil
	}
	key := strings.TrimSpace(result.RateLimits.LimitID)
	if key == "" {
		key = "codex"
	}
	return map[string]RateLimitSnapshot{key: *result.RateLimits}
}

func readRPCResponse(ctx context.Context, scanner *bufio.Scanner, id int) (rpcEnvelope, error) {
	for {
		envelope, err := readNextRPCEnvelope(ctx, scanner)
		if err != nil {
			return rpcEnvelope{}, err
		}
		if rpcID(envelope.ID) != id {
			continue
		}
		if envelope.Error != nil {
			return rpcEnvelope{}, fmt.Errorf("Codex app-server request %d failed (%d): %s", id, envelope.Error.Code, envelope.Error.Message)
		}
		return envelope, nil
	}
}

func readNextRPCEnvelope(ctx context.Context, scanner *bufio.Scanner) (rpcEnvelope, error) {
	if scanner.Scan() {
		var envelope rpcEnvelope
		if err := json.Unmarshal(scanner.Bytes(), &envelope); err != nil {
			return rpcEnvelope{}, fmt.Errorf("decode Codex app-server message: %w", err)
		}
		return envelope, nil
	}
	if err := scanner.Err(); err != nil {
		return rpcEnvelope{}, fmt.Errorf("read Codex app-server response: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return rpcEnvelope{}, err
	}
	return rpcEnvelope{}, io.EOF
}

func rpcID(raw json.RawMessage) int {
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	id, _ := strconv.Atoi(text)
	return id
}

func withAppServerStderr(err error, stderr string) error {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return err
	}
	if len(stderr) > 1000 {
		stderr = stderr[:1000] + " [truncated]"
	}
	return fmt.Errorf("%w: %s", err, stderr)
}
