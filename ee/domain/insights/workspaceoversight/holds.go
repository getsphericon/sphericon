package workspaceoversight

import (
	"context"

	"github.com/getsphericon/sphericon/ee/domain/identity/servicecredentials"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
)

// HoldInput is what a setter states about a hold.
type HoldInput = holds.Input

// HoldChange is the outcome of setting or clearing a hold.
type HoldChange = holds.Change

// SetHold sets the hold under key on a Workspace, or renews it when it exists: one call
// does both and repeating it is harmless. It is recorded in the Audit log under the
// credential's name. The rules of a hold are its owner's (holds.Set), with the expiry checked
// against the configured maximum. ErrWorkspaceNotFound for an unknown Workspace.
//
// The service endpoints scope over the one Workspace the call names, after its row is
// loaded (docs/architecture/workspace-scoping.md, list item 7).
func (m *Module) SetHold(ctx context.Context, cred *ent.ServiceCredential, workspaceID, key string, in HoldInput) (HoldChange, error) {
	if err := m.licensed(); err != nil {
		return HoldChange{}, err
	}
	s, err := m.holdScope(ctx, cred, workspaceID)
	if err != nil {
		return HoldChange{}, err
	}
	return holds.Set(ctx, s, key, cred.Name, in, holds.Limits{Now: m.now(), MaxExpiry: m.holdMax})
}

// ClearHold removes the hold under key. Clearing a missing hold succeeds. It is recorded
// in the Audit log under the credential's name. ErrWorkspaceNotFound for an unknown Workspace.
func (m *Module) ClearHold(ctx context.Context, cred *ent.ServiceCredential, workspaceID, key string) (HoldChange, error) {
	if err := m.licensed(); err != nil {
		return HoldChange{}, err
	}
	s, err := m.holdScope(ctx, cred, workspaceID)
	if err != nil {
		return HoldChange{}, err
	}
	return holds.Clear(ctx, s, key)
}

// ActiveHolds lists the holds of a Workspace that are active now, by key.
// ErrWorkspaceNotFound for an unknown Workspace.
func (m *Module) ActiveHolds(ctx context.Context, workspaceID string) ([]*ent.Hold, error) {
	if err := m.licensed(); err != nil {
		return nil, err
	}
	s, err := m.scopeOver(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return m.holdsOf(ctx, s)
}

// holdsOf lists the holds of the Workspace of s that are active now, by key.
func (m *Module) holdsOf(ctx context.Context, s *ent.Scoped) ([]*ent.Hold, error) {
	st, err := holds.Read(ctx, s, m.now(), true)
	if err != nil {
		return nil, err
	}
	return st.Holds(), nil
}

// holdScope loads the Workspace row and scopes over it, acting as the credential.
func (m *Module) holdScope(ctx context.Context, cred *ent.ServiceCredential, workspaceID string) (*ent.Scoped, error) {
	s, err := m.scopeOver(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return m.bus.Act(s, servicecredentials.ServiceActor(cred)), nil
}
