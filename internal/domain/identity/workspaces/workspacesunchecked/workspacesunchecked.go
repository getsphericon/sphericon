// Package workspacesunchecked holds the workspaces module's unchecked operations (ADR 0110), the
// methods of its Module, which each importer builds in its own constructor: the
// restore and the erase of the deletion cycle, which need a Membership that holds the delete
// Permission and every owner's rows removed, and the edit of the name and the postal address,
// whose clearing needs no marketing email armed (ADR 0156). Their checks are the use cases'
// (internal/usecase/workspaceview, which restores and edits one, and
// internal/usecase/workspacedeletion); only
// a use case may import this package. The workspaces module does not import it; it declares the
// cycle's table (workspaces.DeletionCycle), whose restore and erase moves are written here.
package workspacesunchecked

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Module is the workspaces module's unchecked operations, which date their moves by the clock it
// is built with.
type Module struct {
	now func() time.Time
}

// New builds the unchecked operations on the Instance clock now.
func New(now func() time.Time) *Module { return &Module{now: now} }

// Edit is the rename of the Workspace of a locked scope and, when it carries one, the change of its
// postal address, opened on the row and written by ApplyEdit once the use case's check holds:
// clearing a stored address is refused while marketing email is armed (ADR 0156), which the Removal
// check above identity finds, so the use case asks ClearsPostalAddress first.
type Edit struct {
	revision      int64
	name          string
	postalAddress *string
	clears        bool
}

// EditInput is the edit made on Revision (ADR 0119): the Name, and the PostalAddress, which nil
// leaves as it is and "" clears.
type EditInput struct {
	Revision      int64
	Name          string
	PostalAddress *string
}

// EditIn opens the edit under the Workspace lock l: in.Name is held to the name rule
// (validation.Name), and in.PostalAddress, when non-nil, is trimmed, so whitespace alone clears it.
func (m *Module) EditIn(ctx context.Context, l *ent.WorkspaceLocked, in EditInput) (*Edit, error) {
	name, err := validation.Name(in.Name)
	if err != nil {
		return nil, err
	}
	e := &Edit{revision: in.Revision, name: name}
	if in.PostalAddress == nil {
		return e, nil
	}
	stored, err := l.Scope().Workspace(ctx)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(*in.PostalAddress)
	e.postalAddress = &trimmed
	e.clears = trimmed == "" && workspaces.HasPostalAddress(stored)
	return e, nil
}

// ClearsPostalAddress reports whether the edit takes away the address the Workspace has. Saving an
// empty one over none clears nothing.
func (e *Edit) ClearsPostalAddress() bool { return e.clears }

// ApplyEdit writes the edit e, opened under the Workspace lock l, through the scope's tenant edit
// and returns the Workspace as it now reads: workspaces.ErrRevisionChanged when the Workspace has
// moved past the revision, nothing written for an edit that changes nothing, and a change recorded
// as a `workspace.update` Audit entry of the scope's actor (ADR 0022).
func (m *Module) ApplyEdit(ctx context.Context, l *ent.WorkspaceLocked, e *Edit) (*ent.Workspace, error) {
	upd := l.Scope().EditWorkspaceAt(e.revision).Refusing(workspaces.ErrNotFound, workspaces.ErrRevisionChanged).SetName(e.name)
	if e.postalAddress != nil {
		upd.SetPostalAddress(*e.postalAddress)
	}
	return upd.Save(ctx)
}

// RestoreIn moves the Workspace id from pending deletion back to active inside tx, only while
// it is inside its deletion window now, and returns it as it now reads. One past it, already
// erased or not deleted is lifecycle.ErrInvalidTransition and nothing changes.
func (m *Module) RestoreIn(ctx context.Context, tx *ent.Client, id int64) (*ent.Workspace, error) {
	if err := workspaces.DeletionCycle(cycleStorage(tx, id, m.now())).FireCtx(ctx, workspaces.TriggerRestore); err != nil {
		return nil, err
	}
	return tx.Workspace.Get(ctx, id)
}

// TombstoneIn moves the Workspace id from pending deletion to erased inside tx, only once it is
// past its deletion window now: it writes the tombstone and erased_at in one conditional
// update, and reports whether it did. One still inside its window is refused
// (lifecycle.ErrInvalidTransition) and nothing changes. One not pending deletion (never deleted,
// restored, or erased already) is its no-op: it reports false with no error (ADR 0145). The
// caller has removed what the Workspace owned.
func (m *Module) TombstoneIn(ctx context.Context, tx *ent.Client, id int64) (bool, error) {
	err := workspaces.DeletionCycle(cycleStorage(tx, id, m.now())).FireCtx(ctx, workspaces.TriggerErase)
	if !errors.Is(err, lifecycle.ErrInvalidTransition) {
		return err == nil, err
	}
	// The move was not permitted or lost its compare-and-swap, so the state is read again: a
	// Workspace another writer moved meanwhile, as a restore that wrote first, is no longer
	// pending, and one still pending was kept by the window.
	state, err := workspaces.ReadDeletionState(tx, id)(ctx)
	switch {
	case err != nil:
		return false, err
	case state == workspaces.StatePendingDeletion:
		return false, fmt.Errorf("%w: workspace %d is inside its deletion window", lifecycle.ErrInvalidTransition, id)
	default:
		return false, nil
	}
}

// cycleStorage is the cycle's storage of the restore and the erase over c (the raw client or a
// transaction's) for Workspace id. Each write is the compare-and-swap: one conditional UPDATE whose
// WHERE is the state read, plus the window: a restore inside it, a tombstone past it.
func cycleStorage(c *ent.Client, id int64, now time.Time) lifecycle.Storage[workspaces.DeletionState] {
	return lifecycle.Storage[workspaces.DeletionState]{
		Read: workspaces.ReadDeletionState(c, id),
		Write: func(ctx context.Context, was, to workspaces.DeletionState) (bool, error) {
			u := c.Workspace.Update().Where(workspace.ID(id), workspaces.InDeletionState(was))
			switch {
			case was == workspaces.StatePendingDeletion && to == workspaces.StateActive:
				u.Where(workspaces.InsideDeletionWindow(now)).ClearDiscardedAt()
			case was == workspaces.StatePendingDeletion && to == workspaces.StateErased:
				// The tombstone: the row stays, with nothing that identifies the Workspace
				// and a fresh collect key.
				key := credentials.GenerateCollectKey()
				u.Where(workspaces.PastDeletionWindow(now)).SetName("").SetPostalAddress("").SetCollectKey(key).ClearRetentionDays().SetErasedAt(now)
			default:
				return false, fmt.Errorf("workspaces: no move from %s to %s", was, to)
			}
			n, err := u.Save(ctx)
			return n == 1, err
		},
	}
}

// DiffConfirmationsRecorded is the key of the switch of confirmed opt-in's Audit diff that holds
// how many Confirmations the switch recorded (ADR 0013): the grandfathered ones turning it on
// asserts, none when it is turned off.
const DiffConfirmationsRecorded = "confirmations_recorded"

// SwitchConfirmedOptInInput turns confirmed opt-in On or off, when Recorded Confirmations were
// recorded for it.
type SwitchConfirmedOptInInput struct {
	On       bool
	Recorded int
}

// SwitchedConfirmedOptIn is a switch of confirmed opt-in: the Workspace as it now reads, and the
// switch's Audit entry for the caller to record, nil when the switch changed nothing.
type SwitchedConfirmedOptIn struct {
	Workspace *ent.Workspace
	Audit     *events.AuditEntry
}

// SwitchConfirmedOptInIn turns the scoped Workspace's confirmed opt-in on or off in the
// transaction s is bound to. It is unchecked: turning it on is sound only once every address
// mailable now holds a Confirmation, which the contacts context records and identity cannot
// check, so its caller grandfathers them in the same transaction (ADR 0013), and the caller has
// decided the switch on the revision it was made on (workspaces.ConfirmedOptInSwitch) before it
// recorded any. The write is conditioned on the policy alone, not on that revision: a change of
// another setting while the Confirmations were recorded does not undo the switch and leave them
// recorded with no Audit entry. Of two switches to the same state the second waits for the first
// and writes nothing. The write goes through an unaudited scope, and it returns the switch's one
// `workspace.update` Audit entry for the caller to record, carrying in.Recorded, which a change of
// a column alone cannot carry; none, with nothing written, for a switch to the state the Workspace
// is in.
func (m *Module) SwitchConfirmedOptInIn(ctx context.Context, s *ent.Scoped, in SwitchConfirmedOptInInput) (SwitchedConfirmedOptIn, error) {
	after, err := events.Unaudited(s).UpdateWorkspace().
		Where(workspace.RequireConfirmedOptIn(!in.On)).
		SetRequireConfirmedOptIn(in.On).Save(ctx)
	if ent.IsNotFound(err) {
		now, err := s.Workspace(ctx)
		return SwitchedConfirmedOptIn{Workspace: now}, err
	}
	if err != nil {
		return SwitchedConfirmedOptIn{}, err
	}
	diff := map[string]any{
		workspace.FieldRequireConfirmedOptIn: events.Change(!in.On, in.On),
		DiffConfirmationsRecorded:            in.Recorded,
	}
	return SwitchedConfirmedOptIn{
		Workspace: after,
		Audit:     events.WorkspaceAudit(after, s.Actor(), events.ActionWorkspaceUpdate, diff),
	}, nil
}
