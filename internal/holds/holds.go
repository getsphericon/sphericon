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

// ActiveAt is the one rule for an active hold: its expiry is after now. It exists only as
// this SQL predicate; every read, the eager-load of the holds edge included, passes it.
func ActiveAt(now time.Time) predicate.Hold { return enthold.ExpiresAtGT(now) }

// ActiveEdge shapes the load of a Workspace's holds edge (`WithHolds`) to the active holds
// in key order, so a Workspace loaded with its edge needs no Go filter.
func ActiveEdge(now time.Time) func(*ent.HoldQuery) {
	return func(q *ent.HoldQuery) { q.Where(ActiveAt(now)).Order(enthold.ByKey()) }
}

// KeyPendingDeletion is the key of the pending-deletion Hold (ADR 0043).
const KeyPendingDeletion = "pending_deletion"

// Standing is a Workspace's standing: its independent freeze sources, never merged
// (glossary: Workspace suspension and Staff hold are independent reasons). Reason names
// the one a send reports, in priority order: deletion, suspension, Staff holds.
type Standing struct {
	// Deletion is the pending-deletion Hold of a deleted Workspace, nil for a live one.
	// It is not a row: Workspace deletion is its own source and lifts on restore. It
	// lasts to the erase date, the end of the window.
	Deletion *ent.Hold
	// SuspendedAt is when the Workspace was suspended (ADR 0007), nil when it is not.
	SuspendedAt *time.Time
	// Staff are the active Staff holds by key: none without the `staff` license.
	Staff []*ent.Hold
}

// Reason is the reason a send is Held, "" when the Workspace may send.
func (st Standing) Reason() string {
	switch {
	case st.Deletion != nil:
		return ReasonPendingDeletion
	case st.SuspendedAt != nil:
		return ReasonSuspended
	case len(st.Staff) > 0:
		return ReasonStaff
	default:
		return ""
	}
}

// Holds is the list the owner sees: the pending-deletion Hold, if any, then the Staff
// holds. Suspension is not in it: it is the SuspendedAt field.
func (st Standing) Holds() []*ent.Hold {
	if st.Deletion == nil {
		return st.Staff
	}
	return append([]*ent.Hold{st.Deletion}, st.Staff...)
}

// Of is the standing of a Workspace loaded with its holds edge through ActiveEdge (the
// site's list of Workspaces). staffHonoured is the `staff` license: without it the Staff
// holds are ignored, deletion and suspension still count.
func Of(ws *ent.Workspace, staffHonoured bool) Standing {
	st := Standing{SuspendedAt: ws.SuspendedAt}
	if ws.DiscardedAt != nil {
		st.Deletion = &ent.Hold{
			Key:        KeyPendingDeletion,
			ReasonCode: ReasonPendingDeletion,
			Message:    Message(ReasonPendingDeletion),
			SetBy:      "system",
			CreatedAt:  *ws.DiscardedAt,
			ExpiresAt:  workspacedeletion.EraseDate(*ws.DiscardedAt),
		}
	}
	if staffHonoured {
		st.Staff = ws.Edges.Holds
	}
	return st
}

// Read is the standing of the scope's Workspace at now, one query for its row and one for
// its active Staff holds (none are read without the license).
func Read(ctx context.Context, s *ent.Scoped, now time.Time, staffHonoured bool) (Standing, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return Standing{}, err
	}
	if staffHonoured {
		ws.Edges.Holds, err = s.Hold().Query().Where(ActiveAt(now)).Order(enthold.ByKey()).All(ctx)
		if err != nil {
			return Standing{}, err
		}
	}
	return Of(ws, staffHonoured), nil
}

// Freezer answers the send path's "may this Workspace send now" from the standing (it
// satisfies outbound.Freezer, which the edition's wiring returns it as).
type Freezer struct {
	// Now is the clock; nil is time.Now.
	Now func() time.Time
	// Enabled reports whether Staff holds are honoured at all (the `staff` license); nil
	// is always.
	Enabled func() bool
}

// Frozen implements outbound.Freezer.
func (f Freezer) Frozen(ctx context.Context, s *ent.Scoped, _ *ent.Workspace) (string, error) {
	now := time.Now
	if f.Now != nil {
		now = f.Now
	}
	st, err := Read(ctx, s, now(), f.Enabled == nil || f.Enabled())
	if err != nil {
		return "", err
	}
	return st.Reason(), nil
}
