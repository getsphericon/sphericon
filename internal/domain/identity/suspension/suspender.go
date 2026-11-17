package suspension

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// Suspender suspends a Workspace (ADR 0007), the one operation the command line, the Staff console
// and the abuse detector run, lifts a suspension, and sets the platform's other levers over the
// Workspace's sending: the abuse exemption and the Send cap. The suspension publishes
// events.WorkspaceSuspended in its transaction, and the Notices tell the owners (ADR 0105).
type Suspender struct {
	bus        *events.Bus
	workspaces *workspaces.Module
}

// NewSuspender is the Suspender that suspends through bus and writes the Workspace's standing
// through the workspaces module, which dates it.
func NewSuspender(bus *events.Bus, ws *workspaces.Module) *Suspender {
	return &Suspender{bus: bus, workspaces: ws}
}

// Suspend freezes all outbound sending for a Workspace (ADR 0007): it sets suspended_at together
// with who did it and why (in); a blank reason is ErrReasonRequired. The send path reads it as a
// reversible Hold (ADR 0015); login, reads and /collect are unaffected. It reports whether
// anything changed: suspending an already-suspended Workspace is a no-op and publishes nothing, so
// the first actor and reason, the ones the owners were told about, are never overwritten and the
// owners are not told twice. A change is recorded as `workspace.suspend` in the Workspace's Audit
// log and published as events.WorkspaceSuspended, which tells its owners (ADR 0105), in the same
// transaction.
func (s *Suspender) Suspend(ctx context.Context, workspaceID int64, in SuspendInput) (bool, error) {
	changed, err := s.suspend(ctx, byKey(workspaceID), in)
	if err != nil {
		return false, fmt.Errorf("suspend workspace %d: %w", workspaceID, err)
	}
	return changed, nil
}

// SuspendByPublicID is Suspend of the Workspace with this Workspace id (ADR 0043), the address an
// operator types at the command line; an unknown id fails as `workspace "<id>"`.
func (s *Suspender) SuspendByPublicID(ctx context.Context, publicID string, in SuspendInput) (bool, error) {
	changed, err := s.suspend(ctx, s.byPublicID(publicID), in)
	if err != nil {
		return false, fmt.Errorf("suspend: %w", err)
	}
	return changed, nil
}

// Unsuspend lifts a suspension, clearing its attribution, and records `workspace.unsuspend` for
// the actor by. It stamps cleared_at, so the abuse detector (ADR 0026) counts only Events after
// the clearance. It reports whether the Workspace was suspended. Held sends resume on their own:
// the jobs that were snoozed ask Outbound send again.
func (s *Suspender) Unsuspend(ctx context.Context, workspaceID int64, by Actor) (bool, error) {
	changed, err := s.unsuspend(ctx, byKey(workspaceID), by)
	if err != nil {
		return false, fmt.Errorf("unsuspend workspace %d: %w", workspaceID, err)
	}
	return changed, nil
}

// UnsuspendByPublicID is Unsuspend of the Workspace with this Workspace id (ADR 0043), the address
// an operator types at the command line.
func (s *Suspender) UnsuspendByPublicID(ctx context.Context, publicID string, by Actor) (bool, error) {
	changed, err := s.unsuspend(ctx, s.byPublicID(publicID), by)
	if err != nil {
		return false, fmt.Errorf("unsuspend: %w", err)
	}
	return changed, nil
}
