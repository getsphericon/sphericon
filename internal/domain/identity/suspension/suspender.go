package suspension

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/internal/infra/events"
)

// Suspender suspends a Workspace (ADR 0007): the one operation the command line, the Staff
// console and the abuse detector run. The suspension publishes events.WorkspaceSuspended in its
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
