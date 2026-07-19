package game

import "context"

// UnregisteredIdentityTransferer is implemented by the dedicated lifecycle
// client/store boundary. It must atomically preserve both sessions when it
// returns ErrIdentityAmbiguous for a destination conflict.
type UnregisteredIdentityTransferer interface {
	TransferUnregisteredIdentity(context.Context, string, string, string) error
}

// LifecycleManager is a network-local adapter for IRC connection events.
type LifecycleManager struct {
	caseMappings *CaseMappingStore
	registry     *ContinuationRegistry
	transferer   UnregisteredIdentityTransferer
}

func NewLifecycleManager(caseMappings *CaseMappingStore, registry *ContinuationRegistry, transferer UnregisteredIdentityTransferer) *LifecycleManager {
	if caseMappings == nil {
		caseMappings = NewCaseMappingStore()
	}
	return &LifecycleManager{caseMappings: caseMappings, registry: registry, transferer: transferer}
}

func (l *LifecycleManager) OnISupport(networkID string, params []string) {
	l.caseMappings.UpdateFromISupport(networkID, params)
}

func (l *LifecycleManager) OnObservedNickChange(networkID, oldNick, newNick string) {
	if l == nil || l.registry == nil {
		return
	}
	oldFold := l.caseMappings.Fold(networkID, oldNick)
	newFold := l.caseMappings.Fold(networkID, newNick)
	if err := l.registry.Transfer(networkID, oldFold, newFold); err != nil {
		l.registry.InvalidateNick(networkID, oldFold, "identity_conflict")
		l.registry.InvalidateNick(networkID, newFold, "identity_conflict")
		return
	}
	if l.transferer != nil {
		if err := l.transferer.TransferUnregisteredIdentity(context.Background(), networkID, oldFold, newFold); err != nil {
			// The persistent layer is authoritative. Any conflict or uncertainty
			// leaves both saves untouched there and removes unsafe bare contexts.
			l.registry.InvalidateNick(networkID, oldFold, "identity_transfer_failed")
			l.registry.InvalidateNick(networkID, newFold, "identity_transfer_failed")
		}
	}
}

func (l *LifecycleManager) OnUserQuit(networkID, nick string) {
	if l == nil || l.registry == nil {
		return
	}
	l.registry.InvalidateNick(networkID, l.caseMappings.Fold(networkID, nick), "user_quit")
}

func (l *LifecycleManager) OnConnectionClosed(networkID string) {
	if l != nil && l.registry != nil {
		l.registry.ClearNetwork(networkID, "disconnect")
	}
}

func (l *LifecycleManager) OnConnectionEstablished(networkID string) {
	if l != nil && l.registry != nil {
		l.registry.ClearNetwork(networkID, "reconnect")
	}
}
