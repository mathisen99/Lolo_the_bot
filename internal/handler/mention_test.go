package handler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/yourusername/lolo/internal/database"
	"github.com/yourusername/lolo/internal/user"
)

type scriptedMentionEvent struct {
	delay    time.Duration
	response *APIResponse
}

type scriptedMentionAPI struct {
	events []scriptedMentionEvent
}

func (s *scriptedMentionAPI) SendCommand(context.Context, string, []string, string, string, string, string, bool, time.Duration) (*APIResponse, error) {
	return nil, nil
}

func (s *scriptedMentionAPI) SendCommandStream(context.Context, string, []string, string, string, string, string, bool, time.Duration) (<-chan *APIResponse, error) {
	return nil, nil
}

func (s *scriptedMentionAPI) SendMention(context.Context, string, string, string, string, string, string, string, []*database.Message) (*APIResponse, error) {
	return nil, nil
}

func (s *scriptedMentionAPI) SendMentionStream(ctx context.Context, _, _, _, _, _, _, _ string, _ []*database.Message) (<-chan *APIResponse, error) {
	responses := make(chan *APIResponse)
	go func() {
		defer close(responses)
		for _, event := range s.events {
			if event.delay > 0 {
				select {
				case <-time.After(event.delay):
				case <-ctx.Done():
					return
				}
			}
			select {
			case responses <- event.response:
			case <-ctx.Done():
				return
			}
		}
	}()
	return responses, nil
}

func (s *scriptedMentionAPI) CheckHealth(context.Context) (*HealthResponse, error) {
	return nil, nil
}

func (s *scriptedMentionAPI) GetCommands(context.Context) (*CommandsResponse, error) {
	return nil, nil
}

func (s *scriptedMentionAPI) WaitForInflightRequests(time.Duration) bool {
	return true
}

func TestContainsMention(t *testing.T) {
	handler := &MentionHandler{botNick: "Lolo"}

	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{
			name:    "matches plain nick",
			message: "Lolo: hello there",
			want:    true,
		},
		{
			name:    "matches nick case-insensitively",
			message: "lOlO help me",
			want:    true,
		},
		{
			name:    "matches bridge nick exception",
			message: "Lolo/libera: generate examples of c code",
			want:    true,
		},
		{
			name:    "matches bridge nick case-insensitively",
			message: "lolo/LIBERA, test",
			want:    true,
		},
		{
			name:    "does not match other network suffixes",
			message: "Lolo/othernet: test",
			want:    false,
		},
		{
			name:    "does not match nick prefix only",
			message: "Lolo123 can you help?",
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := handler.ContainsMention(tc.message)
			if got != tc.want {
				t.Fatalf("ContainsMention(%q) = %v, want %v", tc.message, got, tc.want)
			}
		})
	}
}

func TestWorkingAcknowledgementIsDelayedAndSentAtMostOnce(t *testing.T) {
	tests := []struct {
		name         string
		events       []scriptedMentionEvent
		wantStatuses []string
	}{
		{
			name: "quick completion cancels acknowledgement",
			events: []scriptedMentionEvent{
				{response: &APIResponse{Status: "processing", Message: "Working..."}},
				{response: &APIResponse{Status: "success", Message: "Done"}},
			},
			wantStatuses: nil,
		},
		{
			name: "long task sends only first acknowledgement",
			events: []scriptedMentionEvent{
				{response: &APIResponse{Status: "processing", Message: "First update"}},
				{response: &APIResponse{Status: "processing", Message: "Second update"}},
				{delay: 40 * time.Millisecond, response: &APIResponse{Status: "success", Message: "Done"}},
			},
			wantStatuses: []string{"First update"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, err := database.New(filepath.Join(t.TempDir(), "lolo.db"))
			if err != nil {
				t.Fatalf("database.New: %v", err)
			}
			defer func() { _ = db.Close() }()

			api := &scriptedMentionAPI{events: tc.events}
			handler := NewMentionHandler(
				api, user.NewManager(db), db, "Lolo", false, false, "", "libera", nil, 20,
			)
			handler.workingAckDelay = 10 * time.Millisecond
			var statuses []string

			response, err := handler.HandleMention(
				context.Background(), "Lolo do this", "alice", "host", "libera", "#chat", "!",
				func(message string) { statuses = append(statuses, message) },
			)
			if err != nil {
				t.Fatalf("HandleMention: %v", err)
			}
			if response != "Done" {
				t.Fatalf("response = %q, want Done", response)
			}
			if len(statuses) != len(tc.wantStatuses) {
				t.Fatalf("statuses = %v, want %v", statuses, tc.wantStatuses)
			}
			for index := range statuses {
				if statuses[index] != tc.wantStatuses[index] {
					t.Fatalf("statuses = %v, want %v", statuses, tc.wantStatuses)
				}
			}
		})
	}
}
