// Package holds reads Staff holds (ADR 0009): the reversible freezes a service sets
// on a Workspace's outbound sending. It is the one place that decides a hold is active,
// so the send path, the site API and the console agree: a hold is active while its
// expiry is after the clock, and a lapsed one is ignored at once, with no cleanup job.
//
// Setting and clearing live in ee/staff (the service endpoints); this package only
// reads, through the scoped client, and holds no raw client.
package holds

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	enthold "github.com/getsphericon/sphericon/ent/hold"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/workspacedeletion"
)

// The Workspace-level reasons a send is Held. They describe the Workspace, not one send, so
// they live with the holds (ADR 0009) and Outbound send reports them. Suspension (ADR 0007)
// is neither a Hold row nor the pending-deletion Hold: it keeps its own state and fields,
// and only its reason code is declared here.
const (
	ReasonSuspended       = "workspace_suspended"
	ReasonPendingDeletion = "workspace_pending_deletion"
	ReasonStaff           = "staff_hold"
)

// Message words a Workspace-level reason for a human, "" for any other reason. Outbound
// send's Detail reads it, so every surface that reports a Held outcome agrees.
func Message(reason string) string {
	switch reason {
	case ReasonSuspended:
		return "sending is suspended for this workspace"
	case ReasonPendingDeletion:
		return "this workspace is deleted and pending erasure"
	case ReasonStaff:
		return "sending is on hold for this workspace"
	default:
		return ""
	}
}

// ActiveAt is the one rule for an active hold: its expiry is after now. Every read
// goes through it, in SQL here and in Go in Live.
func ActiveAt(now time.Time) predicate.Hold { return enthold.ExpiresAtGT(now) }

// KeyPendingDeletion is the key of the pending-deletion Hold (ADR 0043).
const KeyPendingDeletion = "pending_deletion"

// PendingDeletion is the pending-deletion Hold of a deleted Workspace, none for a live one.
// It is not a row: Workspace deletion is its own Hold source beside the Staff holds, and
// lifts when the Workspace is restored. It lasts to the erase date, the end of the window.
// It is the one statement of "a deleted Workspace is held": the send gate and the owner's
// list both read it.
func PendingDeletion(ws *ent.Workspace) []*ent.Hold {
	if ws.DiscardedAt == nil {
		return nil
	}
	return []*ent.Hold{{
		Key:        KeyPendingDeletion,
		ReasonCode: ReasonPendingDeletion,
		Message:    Message(ReasonPendingDeletion),
		SetBy:      "system",
		CreatedAt:  *ws.DiscardedAt,
		ExpiresAt:  workspacedeletion.EraseDate(*ws.DiscardedAt),
	}}
}

// Active lists the holds of the scope's Workspace that are active at now: the
// pending-deletion Hold of a deleted Workspace first, then the Staff holds ordered by
// key. It is what the owner-facing and console reads of a Workspace's holds use.
func Active(ctx context.Context, s *ent.Scoped, now time.Time) ([]*ent.Hold, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	staff, err := s.Hold().Query().
		Where(ActiveAt(now)).
		Order(enthold.ByKey()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return append(PendingDeletion(ws), staff...), nil
}

// Live keeps the holds of rows that are active at now, in their order. It is Active
// over rows already loaded (the eager-loaded edge, where no predicate can be passed), so
// it states ActiveAt's rule in Go: keep it in step.
func Live(rows []*ent.Hold, now time.Time) []*ent.Hold {
	return lo.Filter(rows, func(h *ent.Hold, _ int) bool { return h.ExpiresAt.After(now) })
}

// Freezer answers the send path's "may this Workspace send now" from the active holds
// (it satisfies outbound.Freezer, which the edition's wiring returns it as): any one active
// hold freezes sending, and a second keeps it frozen after the first is cleared.
type Freezer struct {
	// Now is the clock; nil is time.Now.
	Now func() time.Time
	// Enabled reports whether holds are honoured at all (the `staff` license); nil
	// is always.
	Enabled func() bool
}

// Frozen implements outbound.Freezer.
func (f Freezer) Frozen(ctx context.Context, s *ent.Scoped, _ *ent.Workspace) (string, error) {
	if f.Enabled != nil && !f.Enabled() {
		return "", nil
	}
	now := time.Now
	if f.Now != nil {
		now = f.Now
	}
	held, err := s.Hold().Query().Where(ActiveAt(now())).Exist(ctx)
	if err != nil || !held {
		return "", err
	}
	return ReasonStaff, nil
}
