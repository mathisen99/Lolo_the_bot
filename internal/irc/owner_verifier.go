package irc

import (
	"strings"
	"sync"
	"time"
)

const (
	rizonOwnerNick       = "Mathisen"
	nickServStatusAccept = "STATUS Mathisen 3"
)

// OwnerVerifier performs active NickServ STATUS checks for networks where
// hostmask/nick database identity is not safe enough for owner privileges.
type OwnerVerifier struct {
	client  *Client
	timeout time.Duration

	mu      sync.Mutex
	pending chan string
}

func NewOwnerVerifier(client *Client) *OwnerVerifier {
	return &OwnerVerifier{
		client:  client,
		timeout: 5 * time.Second,
	}
}

func (v *OwnerVerifier) Verify(nick string) (bool, error) {
	if nick != rizonOwnerNick {
		return false, nil
	}

	pending := make(chan string, 1)
	v.mu.Lock()
	v.pending = pending
	v.mu.Unlock()
	defer func() {
		v.mu.Lock()
		v.pending = nil
		v.mu.Unlock()
	}()

	if err := v.client.SendMessage("NickServ", "STATUS "+rizonOwnerNick); err != nil {
		return false, err
	}

	select {
	case msg := <-pending:
		return strings.TrimSpace(msg) == nickServStatusAccept, nil
	case <-time.After(v.timeout):
		return false, nil
	}
}

func (v *OwnerVerifier) HandleNotice(source, message string) {
	if !strings.EqualFold(source, "NickServ") {
		return
	}

	v.mu.Lock()
	pending := v.pending
	v.mu.Unlock()

	if pending == nil {
		return
	}

	select {
	case pending <- strings.TrimSpace(message):
	default:
	}
}
