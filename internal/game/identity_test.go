package game

import (
	"context"
	"errors"
	"testing"

	"github.com/yourusername/lolo/internal/database"
)

type identityManagerSpy struct {
	users             map[string]*database.User
	registered        map[string]bool
	registrationCalls int
}

func (s *identityManagerSpy) GetUser(nick string) (*database.User, error) {
	return s.users[nick], nil
}

func (s *identityManagerSpy) IsRegisteredUser(nick string) (bool, error) {
	s.registrationCalls++
	return s.registered[nick], nil
}

func TestIRCCaseMappingCanonicalizationAndISupport(t *testing.T) {
	tests := []struct {
		mapping CaseMapping
		input   string
		want    string
	}{
		{CaseMappingASCII, `AZ[]\^`, `az[]\^`},
		{CaseMappingStrictRFC1459, `AZ[]\^`, `az{}|^`},
		{CaseMappingRFC1459, `AZ[]\^`, `az{}|~`},
	}
	for _, tc := range tests {
		if got := IRCCasefold(tc.input, tc.mapping); got != tc.want {
			t.Fatalf("IRCCasefold(%q, %q) = %q, want %q", tc.input, tc.mapping, got, tc.want)
		}
	}

	store := NewCaseMappingStore()
	if got := store.Get("unknown"); got != CaseMappingASCII {
		t.Fatalf("unknown network default = %q, want ascii", got)
	}
	if got := store.Get("libera"); got != CaseMappingRFC1459 {
		t.Fatalf("libera default = %q, want rfc1459", got)
	}
	if !store.UpdateFromISupport("libera", []string{"Lolo", "CHANTYPES=#", "CASEMAPPING=strict-rfc1459", "are supported"}) {
		t.Fatal("CASEMAPPING was not parsed from 005")
	}
	if got := store.Fold("libera", `Nick^`); got != `nick^` {
		t.Fatalf("strict fold = %q", got)
	}
	if store.UpdateFromISupport("libera", []string{"CASEMAPPING=unicode"}) {
		t.Fatal("unsupported CASEMAPPING was accepted")
	}
	if got := store.Get("libera"); got != CaseMappingStrictRFC1459 {
		t.Fatalf("unsupported update changed mapping to %q", got)
	}
}

func TestIdentityResolverUsesStableRegisteredIDAndNoNicknameFallback(t *testing.T) {
	manager := &identityManagerSpy{
		users:      map[string]*database.User{"Alice": {ID: 42, Nick: "Alice", Level: database.LevelNormal}},
		registered: map[string]bool{"Alice": true},
	}
	policyCalls := 0
	resolver := NewIdentityResolver(manager, NewCaseMappingStore(), RegisteredIdentityPolicyFunc(
		func(_ context.Context, networkID, nick, hostmask string, user *database.User) (bool, error) {
			policyCalls++
			if networkID != "libera" || nick != "Alice" || hostmask != "ident@host" || user.ID != 42 {
				t.Fatal("registered policy received incorrect identity inputs")
			}
			return true, nil
		},
	))
	identity, err := resolver.Resolve(context.Background(), "libera", "Alice", "ident@host")
	if err != nil {
		t.Fatalf("Resolve registered: %v", err)
	}
	if identity.Kind != IdentityRegistered || identity.Value != "42" {
		t.Fatalf("registered identity = %#v", identity)
	}
	if manager.registrationCalls != 1 || policyCalls != 1 {
		t.Fatalf("guard calls=%d policy calls=%d", manager.registrationCalls, policyCalls)
	}

	ambiguous := NewIdentityResolver(manager, NewCaseMappingStore(), RegisteredIdentityPolicyFunc(
		func(context.Context, string, string, string, *database.User) (bool, error) { return false, nil },
	))
	identity, err = ambiguous.Resolve(context.Background(), "libera", "Alice", "ident@host")
	if !errors.Is(err, ErrIdentityAmbiguous) || identity != (SessionIdentity{}) {
		t.Fatalf("ambiguous registered identity = %#v, %v", identity, err)
	}
	if identity.Kind == IdentityUnregistered {
		t.Fatal("registered ambiguity fell back to nickname identity")
	}
}

func TestIdentityResolverUnregisteredPathNeverInvokesRegisteredWHOISPolicy(t *testing.T) {
	manager := &identityManagerSpy{users: map[string]*database.User{}, registered: map[string]bool{}}
	resolver := NewIdentityResolver(manager, NewCaseMappingStore(), RegisteredIdentityPolicyFunc(
		func(context.Context, string, string, string, *database.User) (bool, error) {
			panic("WHOIS/NickServ policy invoked for unregistered user")
		},
	))
	identity, err := resolver.Resolve(context.Background(), "rizon", "[Guest]^", "private@hostmask")
	if err != nil {
		t.Fatalf("Resolve unregistered: %v", err)
	}
	if identity.Kind != IdentityUnregistered || identity.Value != "{guest}~" {
		t.Fatalf("unregistered identity = %#v", identity)
	}
	if manager.registrationCalls != 0 {
		t.Fatalf("unregistered path entered registered guard %d times", manager.registrationCalls)
	}
}

func TestIdentityResolverGuardPrecedesRegisteredPolicy(t *testing.T) {
	manager := &identityManagerSpy{
		users:      map[string]*database.User{"Alice": {ID: 42, Nick: "Alice"}},
		registered: map[string]bool{"Alice": false},
	}
	resolver := NewIdentityResolver(manager, NewCaseMappingStore(), RegisteredIdentityPolicyFunc(
		func(context.Context, string, string, string, *database.User) (bool, error) {
			panic("registered policy bypassed IsRegisteredUser guard")
		},
	))
	_, err := resolver.Resolve(context.Background(), "libera", "Alice", "ident@host")
	if !errors.Is(err, ErrIdentityAmbiguous) {
		t.Fatalf("Resolve error = %v, want identity ambiguity", err)
	}
}
