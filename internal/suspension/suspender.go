package suspension

import (
	"context"
	"errors"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/ownernotice"
)

// ErrNoticeNotSent means a Workspace was suspended but its owners could not be told: the
// suspension stands.
var ErrNoticeNotSent = errors.New("workspace suspended, but the owner notice could not be sent")

// Suspender suspends a Workspace and tells its owners (ADR 0007): the one sequence the command
// line, the Staff console and the abuse detector run.
type Suspender struct {
	bus     *events.Bus
	client  *ent.Client
	notify  ownernotice.Notify
	support string
}

// NewSuspender is the Suspender that suspends through bus and tells the owners through notify
// (the platform's system sender, nil for none), giving supportEmail as where they appeal.
func NewSuspender(bus *events.Bus, client *ent.Client, notify ownernotice.Notify, supportEmail string) *Suspender {
	return &Suspender{bus: bus, client: client, notify: notify, support: supportEmail}
}

// Suspend suspends the Workspace as by and tells its owners why. It reports whether anything
// changed: an already-suspended Workspace is left as it was and its owners are not told twice.
// A notice that cannot be sent does not undo the suspension: it reports true with an error
// that wraps ErrNoticeNotSent.
func (s *Suspender) Suspend(ctx context.Context, workspaceID int64, by Actor, reason string) (bool, error) {
	changed, err := SuspendWorkspace(ctx, s.bus, workspaceID, by, reason)
	if err != nil || !changed {
		return changed, err
	}
	if err := NotifyOwners(ctx, s.client, s.notify, s.support, workspaceID); err != nil {
		return true, fmt.Errorf("%w: %w", ErrNoticeNotSent, err)
	}
	return true, nil
}
