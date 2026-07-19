package irc

import (
	"testing"

	ircproto "gopkg.in/irc.v4"
)

type gameLifecycleSpy struct {
	isupport    [][]string
	nickChanges [][2]string
	quits       []string
	closed      int
	established int
}

func (s *gameLifecycleSpy) OnISupport(_ string, params []string) {
	s.isupport = append(s.isupport, append([]string(nil), params...))
}
func (s *gameLifecycleSpy) OnObservedNickChange(_, oldNick, newNick string) {
	s.nickChanges = append(s.nickChanges, [2]string{oldNick, newNick})
}
func (s *gameLifecycleSpy) OnUserQuit(_, nick string)      { s.quits = append(s.quits, nick) }
func (s *gameLifecycleSpy) OnConnectionClosed(string)      { s.closed++ }
func (s *gameLifecycleSpy) OnConnectionEstablished(string) { s.established++ }

func TestConnectionEmitsGameIdentityLifecycleHooks(t *testing.T) {
	cm, _, cleanup := newTestConnectionManager(t)
	defer cleanup()
	spy := &gameLifecycleSpy{}
	cm.SetGameLifecycleHandler(spy)

	cm.handleMessage(nil, &ircproto.Message{Command: "005", Params: []string{"Lolo", "CASEMAPPING=strict-rfc1459", "are supported"}})
	cm.handleNickChange(&ircproto.Message{Prefix: &ircproto.Prefix{Name: "Alice", User: "ident", Host: "host"}, Params: []string{"Alice_"}})
	cm.handleQuit(&ircproto.Message{Prefix: &ircproto.Prefix{Name: "Alice_", User: "ident", Host: "host"}})
	cm.handleMessage(nil, &ircproto.Message{Command: "ERROR", Params: []string{"connection lost"}})

	if len(spy.isupport) != 1 || len(spy.nickChanges) != 1 || spy.nickChanges[0] != [2]string{"Alice", "Alice_"} {
		t.Fatalf("lifecycle hooks = %#v %#v", spy.isupport, spy.nickChanges)
	}
	if len(spy.quits) != 1 || spy.quits[0] != "Alice_" {
		t.Fatalf("quit hooks = %#v", spy.quits)
	}
	if spy.closed != 1 {
		t.Fatalf("connection closed hooks = %d", spy.closed)
	}
}
