package irc

import (
	"sync"
	"testing"
	"time"
)

type ownerStatusSenderSpy struct {
	mu     sync.Mutex
	sent   []string
	notify chan struct{}
}

func newOwnerStatusSenderSpy() *ownerStatusSenderSpy {
	return &ownerStatusSenderSpy{notify: make(chan struct{}, 8)}
}

func (s *ownerStatusSenderSpy) SendMessage(target, message string) error {
	s.mu.Lock()
	s.sent = append(s.sent, target+"\x00"+message)
	s.mu.Unlock()
	s.notify <- struct{}{}
	return nil
}

func (s *ownerStatusSenderSpy) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sent)
}

func verifyWithNotice(t *testing.T, responseSource, response string) bool {
	t.Helper()
	sender := newOwnerStatusSenderSpy()
	verifier := NewOwnerVerifier(sender)
	verifier.timeout = 50 * time.Millisecond
	result := make(chan bool, 1)
	go func() {
		verified, err := verifier.Verify("Mathisen")
		if err != nil {
			result <- false
			return
		}
		result <- verified
	}()
	select {
	case <-sender.notify:
	case <-time.After(time.Second):
		t.Fatal("NickServ STATUS request was not sent")
	}
	verifier.HandleNotice(responseSource, response)
	select {
	case verified := <-result:
		return verified
	case <-time.After(time.Second):
		t.Fatal("owner verification did not finish")
		return false
	}
}

func TestRizonOwnerVerifierRequiresExactNickAndFreshExactStatus(t *testing.T) {
	for _, nick := range []string{"mathisen", "MATHISEN", "Mathisen_", "ident@host", "trusted/vhost"} {
		t.Run("nick alone "+nick, func(t *testing.T) {
			sender := newOwnerStatusSenderSpy()
			verifier := NewOwnerVerifier(sender)
			verifier.timeout = 5 * time.Millisecond
			verified, err := verifier.Verify(nick)
			if err != nil || verified {
				t.Fatalf("Verify(%q) = %v, %v; want false, nil", nick, verified, err)
			}
			if sender.count() != 0 {
				t.Fatalf("non-exact nick %q caused a NickServ request", nick)
			}
		})
	}

	invalidNotices := []struct {
		name, source, response string
	}{
		{name: "legacy STATUS prefix", source: "NickServ", response: "STATUS Mathisen 3"},
		{name: "different account", source: "NickServ", response: "Other 3"},
		{name: "not identified", source: "NickServ", response: "Mathisen 2"},
		{name: "extra whitespace", source: "NickServ", response: " Mathisen 3 "},
		{name: "untrusted source", source: "Mallory", response: "Mathisen 3"},
	}
	for _, tc := range invalidNotices {
		t.Run(tc.name, func(t *testing.T) {
			if verifyWithNotice(t, tc.source, tc.response) {
				t.Fatalf("notice %q from %q incorrectly granted owner", tc.response, tc.source)
			}
		})
	}

	if !verifyWithNotice(t, "NickServ", "Mathisen 3") {
		t.Fatal("fresh exact NickServ response did not grant owner")
	}

	// A matching notice observed before the request is not fresh and cannot be
	// replayed for a later owner check.
	sender := newOwnerStatusSenderSpy()
	verifier := NewOwnerVerifier(sender)
	verifier.timeout = 10 * time.Millisecond
	verifier.HandleNotice("NickServ", "Mathisen 3")
	verified, err := verifier.Verify("Mathisen")
	if err != nil || verified {
		t.Fatalf("stale notice replay = %v, %v; want false, nil", verified, err)
	}
}
