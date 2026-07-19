package game

import (
	"context"
	"fmt"
	"strconv"

	"github.com/yourusername/lolo/internal/database"
)

// IdentityUserManager is deliberately split into lookup and registration
// guard operations. Resolve always calls IsRegisteredUser before entering any
// registered-user policy that may perform WHOIS or NickServ work.
type IdentityUserManager interface {
	GetUser(nick string) (*database.User, error)
	IsRegisteredUser(nick string) (bool, error)
}

// RegisteredIdentityPolicy adapts Lolo's existing network-specific identity
// policy. Implementations may perform registered-user WHOIS/NickServ checks;
// Resolve invokes them only after IsRegisteredUser returned true.
type RegisteredIdentityPolicy interface {
	EstablishRegisteredIdentity(context.Context, string, string, string, *database.User) (bool, error)
}

type RegisteredIdentityPolicyFunc func(context.Context, string, string, string, *database.User) (bool, error)

func (f RegisteredIdentityPolicyFunc) EstablishRegisteredIdentity(ctx context.Context, networkID, nick, hostmask string, user *database.User) (bool, error) {
	return f(ctx, networkID, nick, hostmask, user)
}

// IdentityResolver creates only canonical, network-scoped game identities.
type IdentityResolver struct {
	users       IdentityUserManager
	caseMapping *CaseMappingStore
	registered  RegisteredIdentityPolicy
}

func NewIdentityResolver(users IdentityUserManager, caseMapping *CaseMappingStore, registered RegisteredIdentityPolicy) *IdentityResolver {
	if caseMapping == nil {
		caseMapping = NewCaseMappingStore()
	}
	return &IdentityResolver{users: users, caseMapping: caseMapping, registered: registered}
}

// Resolve never returns or persists hostmask. Hostmask is supplied only to the
// existing registered-user policy and is unreachable for unregistered users.
func (r *IdentityResolver) Resolve(ctx context.Context, networkID, nick, hostmask string) (SessionIdentity, error) {
	if r == nil || r.users == nil || !networkPattern.MatchString(networkID) || nick == "" {
		return SessionIdentity{}, ErrIdentityAmbiguous
	}
	user, err := r.users.GetUser(nick)
	if err != nil {
		return SessionIdentity{}, fmt.Errorf("%w: user lookup failed: %v", ErrIdentityAmbiguous, err)
	}
	if user == nil {
		identity := SessionIdentity{Kind: IdentityUnregistered, Value: r.caseMapping.Fold(networkID, nick)}
		if err := identity.Validate(); err != nil {
			return SessionIdentity{}, fmt.Errorf("%w: invalid unregistered identity", ErrIdentityAmbiguous)
		}
		return identity, nil
	}

	// This guard is mandatory even though GetUser just returned a row: it keeps
	// the safety invariant explicit at the exact boundary before policy work.
	registered, err := r.users.IsRegisteredUser(nick)
	if err != nil || !registered || r.registered == nil || user.ID <= 0 {
		return SessionIdentity{}, ErrIdentityAmbiguous
	}
	established, err := r.registered.EstablishRegisteredIdentity(ctx, networkID, nick, hostmask, user)
	if err != nil {
		return SessionIdentity{}, fmt.Errorf("%w: registered identity policy failed: %v", ErrIdentityAmbiguous, err)
	}
	if !established {
		return SessionIdentity{}, ErrIdentityAmbiguous
	}
	return SessionIdentity{Kind: IdentityRegistered, Value: strconv.FormatInt(user.ID, 10)}, nil
}

func (r *IdentityResolver) FoldNick(networkID, nick string) string {
	if r == nil || r.caseMapping == nil {
		return IRCCasefold(nick, DefaultCaseMapping(networkID))
	}
	return r.caseMapping.Fold(networkID, nick)
}

func ContinuationKeyFor(networkID string, identity SessionIdentity) (ContinuationKey, error) {
	key := ContinuationKey{NetworkID: networkID, IdentityKind: identity.Kind, IdentityValue: identity.Value}
	if err := key.Validate(); err != nil {
		return ContinuationKey{}, err
	}
	return key, nil
}
