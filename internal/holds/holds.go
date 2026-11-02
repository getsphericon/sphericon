// Package holds reads Staff holds (ADR 0009): the reversible freezes a service sets
// on a Workspace's outbound sending. It is the one place that decides a hold is active,
// so the send path, the site API and the console agree: a hold is active while its
// expiry is after the clock, and a lapsed one is ignored at once, with no cleanup job.
//
// It also sets (idempotently) and clears a hold for the service endpoints in ee/staff, which
// only authorize and validate. It works through the scoped client and holds no raw client.
package holds

import (
	"context"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	enthold "github.com/getsphericon/sphericon/ent/hold"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/holdreason"
	"github.com/getsphericon/sphericon/internal/workspacedeletion"
)

// The Workspace-level reasons a send is Held. They describe the Workspace, not one send, so
// they live with the holds (ADR 0009) and Outbound send reports them. Suspension (ADR 0007)
// is neither a Hold row nor the pending-deletion Hold: it keeps its own state and fields,
// and only its reason code is declared here.
const (
	ReasonSuspended       = string(holdreason.WorkspaceSuspended)
	ReasonPendingDeletion = string(holdreason.WorkspacePendingDeletion)
	ReasonStaff           = string(holdreason.StaffHold)
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
// this SQL predicate; every read passes it.
func ActiveAt(now time.Time) predicate.Hold { return enthold.ExpiresAtGT(now) }

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

// standingOf is the standing of a Workspace row and its active Staff holds, which are
// ignored when staffHonoured is false (the `staff` license): deletion and suspension
// still count.
func standingOf(ws *ent.Workspace, staff []*ent.Hold, staffHonoured bool) Standing {
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
		st.Staff = staff
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
	var staff []*ent.Hold
	if staffHonoured {
		staff, err = s.Hold().Query().Where(ActiveAt(now)).Order(enthold.ByKey()).All(ctx)
		if err != nil {
			return Standing{}, err
		}
	}
	return standingOf(ws, staff, staffHonoured), nil
}

// Holds answers the send path's "may this Workspace send now" from the standing. The one
// thing that varies between editions is whether Staff holds are honoured (the `staff`
// license); it is the one function the constructor takes.
type Holds struct {
	honoured func() bool
	now      func() time.Time
}

// New builds the module. honoured reports whether Staff holds count at all (deletion and
// suspension always do); now is the clock a hold's expiry is read against.
func New(honoured func() bool, now func() time.Time) *Holds {
	return &Holds{honoured: honoured, now: now}
}

// Standing is the standing of the scope's Workspace now, as this instance honours it.
func (h *Holds) Standing(ctx context.Context, s *ent.Scoped) (Standing, error) {
	return Read(ctx, s, h.now(), h.honoured())
}

// Reason is the reason a send is Held for the scope's Workspace, "" when it may send.
func (h *Holds) Reason(ctx context.Context, s *ent.Scoped) (string, error) {
	st, err := h.Standing(ctx, s)
	if err != nil {
		return "", fmt.Errorf("holds: freeze check: %w", err)
	}
	return st.Reason(), nil
}
