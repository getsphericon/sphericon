package suspension

import (
	"context"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/internal/infra/events"
)

// Suspender suspends a Workspace (ADR 0007): the one operation the command line, the Staff
// console and the abuse detector run, and lifts a suspension by the Workspace's id for the command
// line. The suspension publishes events.WorkspaceSuspended in its
// transaction, and the Notices tell the owners (ADR 0105).
type Suspender struct {
	bus *events.Bus
	now func() time.Time
}

// NewSuspender is the Suspender that suspends through bus on the Instance clock now.
func NewSuspender(bus *events.Bus, now func() time.Time) *Suspender {
	return &Suspender{bus: bus, now: now}
}

// Suspend suspends the Workspace as by. It reports whether anything changed: an
// already-suspended Workspace is left as it was, and nothing is published for it, so its owners
// are not told twice.
func (s *Suspender) Suspend(ctx context.Context, workspaceID int64, by Actor, reason string) (bool, error) {
	return SuspendWorkspace(ctx, s.bus, workspaceID, by, reason, s.now())
}

// SuspendByPublicID is Suspend of the Workspace with this Workspace id (ADR 0043), the address an
// operator types at the command line; an unknown id fails as `workspace "<id>"`.
func (s *Suspender) SuspendByPublicID(ctx context.Context, publicID string, by Actor, reason string) (bool, error) {
	changed, err := suspend(ctx, s.bus, byPublicID(publicID), by, reason, s.now())
	if err != nil {
		return false, fmt.Errorf("suspend: %w", err)
	}
	return changed, nil
}

// UnsuspendByPublicID lifts the suspension of the Workspace with this Workspace id as by, and
// reports whether it was suspended. Held sends resume on their own.
func (s *Suspender) UnsuspendByPublicID(ctx context.Context, publicID string, by Actor) (bool, error) {
	changed, err := unsuspend(ctx, s.bus, byPublicID(publicID), by, s.now())
	if err != nil {
		return false, fmt.Errorf("unsuspend: %w", err)
	}
	return changed, nil
}
