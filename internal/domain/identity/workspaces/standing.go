package workspaces

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/ent/workspacestanding"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The Workspace's sending standing lives on its row: the suspension, the abuse exemption, the
// Send cap (internal/domain/identity/suspension says what they mean and audits each change) and the markers of the
// abuse detector's warning and of the suspension's notice (ee/domain/insights/abusedetector,
// internal/domain/identity/noticedelivery). These functions
// are their writes; each runs on the client or transaction it is given.

// standing is the suspension state of a Workspace (ADR 0026), kept as suspended_at: a machine
// (ADR 0109) whose two moves are Suspend and Unsuspend.
type standing string

const (
	standingActive    standing = "active"
	standingSuspended standing = "suspended"
)

type standingTrigger string

const (
	triggerSuspend   standingTrigger = "suspend"
	triggerUnsuspend standingTrigger = "unsuspend"
)

// suspension is what a Suspend writes with the moment: who did it and why.
type suspension struct {
	kind   workspace.SuspendedByKind
	byID   *string
	reason string
}

// moveStanding fires trigger on the standing of the Workspace id inside tx and reports whether it
// moved. A Workspace already in the state the trigger leads to stays as it is, its first actor and
// reason standing, and so does one that a concurrent move changed first or that does not exist.
func moveStanding(ctx context.Context, tx *ent.Client, id int64, trigger standingTrigger, by suspension, at time.Time) (bool, error) {
	moved := false
	machine := lifecycle.New(lifecycle.Storage[standing]{
		Read: func(ctx context.Context) (standing, error) {
			row, err := tx.Workspace.Query().Where(workspace.ID(id)).Select(workspace.FieldSuspendedAt).Only(ctx)
			if err != nil {
				return "", err
			}
			if row.SuspendedAt != nil {
				return standingSuspended, nil
			}
			return standingActive, nil
		},
		Write: func(ctx context.Context, from, to standing) (bool, error) {
			upd := tx.Workspace.Update().Where(workspace.ID(id))
			if from == standingSuspended {
				upd.Where(workspace.SuspendedAtNotNil())
			} else {
				upd.Where(workspace.SuspendedAtIsNil())
			}
			if to == standingSuspended {
				upd.SetSuspendedAt(at).SetSuspendedByKind(by.kind).SetNillableSuspendedByID(by.byID).
					SetSuspensionReason(by.reason)
			} else {
				upd.ClearSuspendedAt().ClearSuspendedByKind().ClearSuspendedByID().ClearSuspensionReason().
					SetClearedAt(at)
			}
			n, err := upd.Save(ctx)
			moved = n == 1
			return moved, err
		},
	})
	machine.Configure(standingActive).Permit(triggerSuspend, standingSuspended).Ignore(triggerUnsuspend)
	machine.Configure(standingSuspended).Permit(triggerUnsuspend, standingActive).Ignore(triggerSuspend)
	err := machine.FireCtx(ctx, trigger)
	if ent.IsNotFound(err) || errors.Is(err, lifecycle.ErrInvalidTransition) {
		return false, nil
	}
	return moved, err
}

// SuspendInput is who suspends a Workspace (Kind, and ByID for a Staff member) and why.
type SuspendInput struct {
	Kind   workspace.SuspendedByKind
	ByID   *string
	Reason string
}

// SuspendIn suspends the Workspace id inside tx while it is not suspended: it sets suspended_at to
// now with who did it and why (in). It reports whether it did: false when the Workspace was
// already suspended, whose first actor and reason stand.
func (m *Module) SuspendIn(ctx context.Context, tx *ent.Client, id int64, in SuspendInput) (bool, error) {
	return moveStanding(ctx, tx, id, triggerSuspend, suspension{kind: in.Kind, byID: in.ByID, reason: in.Reason}, m.now())
}

// UnsuspendIn lifts the suspension of the Workspace id inside tx while it is suspended: it clears
// the suspension and its attribution and stamps cleared_at now. It reports whether it did:
// false when the Workspace was not suspended.
func (m *Module) UnsuspendIn(ctx context.Context, tx *ent.Client, id int64) (bool, error) {
	return moveStanding(ctx, tx, id, triggerUnsuspend, suspension{}, m.now())
}

// SetAbuseExemptIn sets the abuse exemption of the Workspace id inside tx while it differs, and
// returns how many rows it changed.
func (m *Module) SetAbuseExemptIn(ctx context.Context, tx *ent.Client, id int64, exempt bool) (int, error) {
	return tx.Workspace.Update().
		Where(workspace.ID(id), workspace.AbuseExempt(!exempt)).
		SetAbuseExempt(exempt).
		Save(ctx)
}

// SetSendCapIn sets the Send cap of the Workspace id inside tx, or clears it with nil. A Send cap
// outside the contract's bounds is refused (CheckSendCap).
func (m *Module) SetSendCapIn(ctx context.Context, tx *ent.Client, id int64, sendCap *int) error {
	if err := CheckSendCap(sendCap); err != nil {
		return err
	}
	upd := tx.Workspace.UpdateOneID(id)
	if sendCap == nil {
		upd.ClearSendCap()
	} else {
		upd.SetSendCap(*sendCap)
	}
	return upd.Exec(ctx)
}

// CheckSendCap refuses a Send cap outside the contract's bounds (bounds.MaxPerDay) on the field
// `perDay`; nil, no Send cap, passes. Every write of a Workspace's Send cap checks it.
func CheckSendCap(sendCap *int) error {
	if sendCap == nil {
		return nil
	}
	return validation.Range(fieldname.PerDay, *sendCap, bounds.MaxPerDayMin, bounds.MaxPerDayMax)
}

// MarkAbuseFreezeNotified stamps, now, that the Owners of the Workspace id were told of its
// suspension, through c.
func (m *Module) MarkAbuseFreezeNotified(ctx context.Context, c *ent.Client, id int64) error {
	return wroteStanding(standingOf(c, id).SetAbuseFreezeNotifiedAt(m.now()).Save(ctx))
}

// MarkAbuseWarned records, now, that the Owners of the Workspace id were warned of a breach,
// through c.
func (m *Module) MarkAbuseWarned(ctx context.Context, c *ent.Client, id int64) error {
	return wroteStanding(standingOf(c, id).SetAbuseWarningNotifiedAt(m.now()).Save(ctx))
}

// ClearAbuseWarning ends the warning episode of the Workspace id, through c.
func (m *Module) ClearAbuseWarning(ctx context.Context, c *ent.Client, id int64) error {
	return wroteStanding(standingOf(c, id).ClearAbuseWarningNotifiedAt().Save(ctx))
}

// ClearAbuseWarnings ends the warning episode of every Workspace that is warned and matches which,
// through c.
func (m *Module) ClearAbuseWarnings(ctx context.Context, c *ent.Client, which ...predicate.Workspace) error {
	return c.WorkspaceStanding.Update().
		Where(workspacestanding.AbuseWarningNotifiedAtNotNil(), workspacestanding.HasWorkspaceWith(which...)).
		ClearAbuseWarningNotifiedAt().
		Exec(ctx)
}

// AbuseWarned matches the Workspaces whose Owners were warned of the current breach.
func AbuseWarned() predicate.Workspace {
	return workspace.HasStandingWith(workspacestanding.AbuseWarningNotifiedAtNotNil())
}

// Feed is the service feed's write of the Workspaces' positions (ADR 0062): what the hook of
// internal/domain/identity/workspacefeed calls, which takes the position under its lock and holds
// no module, so the type needs nothing.
type Feed struct{}

// NewFeed builds the feed's write.
func NewFeed() *Feed { return &Feed{} }

// SetPosition gives the Workspaces ids the service feed position, through c. The hook runs on the
// creation of a Workspace too, before Creation.CreateIn makes its standing, so the write makes the
// standing of a Workspace that has none yet.
func (f *Feed) SetPosition(ctx context.Context, c *ent.Client, position int64, ids ...int64) error {
	builders := make([]*ent.WorkspaceStandingCreate, len(ids))
	for i, id := range ids {
		builders[i] = c.WorkspaceStanding.Create().SetWorkspaceID(id).SetFeedPosition(position)
	}
	return c.WorkspaceStanding.CreateBulk(builders...).
		OnConflictColumns(workspacestanding.FieldWorkspaceID).
		Update(func(u *ent.WorkspaceStandingUpsert) { u.UpdateFeedPosition() }).
		Exec(ctx)
}

// TopPosition is the highest service feed position any Workspace holds, read through c, 0 when
// none took one (on a new Instance, the first Workspace has no standing yet when it takes its
// position).
func (f *Feed) TopPosition(ctx context.Context, c *ent.Client) (int64, error) {
	top, err := c.WorkspaceStanding.Query().
		Order(ent.Desc(workspacestanding.FieldFeedPosition)).Limit(1).
		Select(workspacestanding.FieldFeedPosition).Ints(ctx)
	if err != nil || len(top) == 0 {
		return 0, err
	}
	return int64(top[0]), nil
}

// StandingOf is what the service keeps about ws for itself (ADR 0168), read through its standing
// edge, which a read that shows or judges it loads (WithStanding). Every Workspace has the row, made
// with it: an edge not loaded, or loaded empty, is a reader's bug, and reading it as "never warned"
// would warn the owners twice, so it panics.
func StandingOf(ws *ent.Workspace) *ent.WorkspaceStanding {
	standing, err := ws.Edges.StandingOrErr()
	if err != nil {
		panic(fmt.Sprintf("workspaces: Workspace %d read without its standing: %v", ws.ID, err))
	}
	return standing
}

// standingIn reads the standing of the Workspace id (ADR 0168).
func standingIn(ctx context.Context, client *ent.Client, id int64) (*ent.WorkspaceStanding, error) {
	return client.WorkspaceStanding.Query().Where(workspacestanding.WorkspaceID(id)).Only(ctx)
}

// wroteStanding is the error of a write of one Workspace's standing: one that found no row is one,
// since every Workspace has its row.
func wroteStanding(n int, err error) error {
	if err == nil && n != 1 {
		return fmt.Errorf("workspaces: the write of a standing found %d rows, not 1", n)
	}
	return err
}

// standingOf is the update of the standing of the Workspace id (ADR 0168).
func standingOf(client *ent.Client, id int64) *ent.WorkspaceStandingUpdate {
	return client.WorkspaceStanding.Update().Where(workspacestanding.WorkspaceID(id))
}
