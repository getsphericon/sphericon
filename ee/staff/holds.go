package staff

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/holds"
)

// DefaultHoldMaxExpiry is how far ahead a hold's expiry may be set when the instance
// does not say otherwise: a service that stops renewing frees the Workspace by then.
const DefaultHoldMaxExpiry = 30 * 24 * time.Hour

// ErrHoldExpiryInPast refuses a hold that would already have lapsed.
var ErrHoldExpiryInPast = errors.New("staff: a hold's expiry must be in the future")

// ErrHoldActionURL refuses an action URL that is not an absolute https URL: it is shown
// to the Workspace's Owners as a link, so no other scheme and no credentials in it.
var ErrHoldActionURL = errors.New("staff: a hold's action URL must be an absolute https URL")

// HoldTooLongError refuses a hold whose expiry is beyond the configured maximum.
type HoldTooLongError struct{ Max time.Duration }

func (e *HoldTooLongError) Error() string {
	return fmt.Sprintf("staff: a hold's expiry may be at most %s from now", e.Max)
}

// HoldInput is what a setter states about a hold.
type HoldInput = holds.Input

// HoldChange is the outcome of setting or clearing a hold.
type HoldChange = holds.Change

// SetHold sets the hold under key on a Workspace, or renews it when it exists: one call
// does both and repeating it is harmless. It is recorded in the Audit log under the
// credential's name. The expiry is required to be ahead of now and no further than the
// configured maximum. ent.IsNotFound for an unknown Workspace.
//
// The service endpoints scope over the one Workspace the call names, after its row is
// loaded (docs/architecture.md, list item 7).
func (m *Module) SetHold(ctx context.Context, cred *ent.ServiceCredential, workspaceID, key string, in HoldInput) (HoldChange, error) {
	if err := m.licensed(); err != nil {
		return HoldChange{}, err
	}
	now := m.now()
	if !in.ExpiresAt.After(now) {
		return HoldChange{}, ErrHoldExpiryInPast
	}
	if in.ExpiresAt.After(now.Add(m.holdMax)) {
		return HoldChange{}, &HoldTooLongError{Max: m.holdMax}
	}
	if in.ActionURL != nil {
		if u, err := url.Parse(*in.ActionURL); err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return HoldChange{}, ErrHoldActionURL
		}
	}
	s, err := m.holdScope(ctx, cred, workspaceID)
	if err != nil {
		return HoldChange{}, err
	}
	return holds.Set(ctx, s, key, cred.Name, in)
}

// ClearHold removes the hold under key. Clearing a missing hold succeeds. It is recorded
// in the Audit log under the credential's name. ent.IsNotFound for an unknown Workspace.
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

// ActiveHolds lists the holds of a Workspace that are active now, by key. ent.IsNotFound
// for an unknown Workspace.
func (m *Module) ActiveHolds(ctx context.Context, workspaceID string) ([]*ent.Hold, error) {
	if err := m.licensed(); err != nil {
		return nil, err
	}
	s, err := m.scopeOver(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
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
	return m.bus.Act(s, ServiceActor(cred)), nil
}
