// Package segmentsunchecked holds the segments module's unchecked save (ADR 0110): a Segment's
// Condition may name, in an event test's filter, a Broadcast or an Automation (ADR 0153), which
// only a module above contacts can answer for, so that check is the use case
// internal/usecase/segmentsaving's, the one kind of package easement lets import this one, and
// which builds its Module. Every other rule of the Segment's own row stays here: its name, its
// Condition (segments.Module.Validate), its revision and its archive. The segments module does not
// import it.
package segmentsunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/segment"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Module is the segments module's unchecked save (docs/architecture/modules.md, the unchecked
// operations).
type Module struct {
	bus      *events.Bus
	segments *segments.Module
}

// New builds the unchecked save over the bus, which runs a save in one transaction under the
// Workspace's lock (ADR 0099), and the segments module, whose rules a save keeps.
func New(bus *events.Bus, segs *segments.Module) *Module {
	return &Module{bus: bus, segments: segs}
}

// Create validates the name and the condition and stores the Segment, without asking whether the
// Broadcasts and Automations its event tests name are the Workspace's: that is the use case's. It
// holds the Workspace's lock (ADR 0099): the Condition may name a Tag or test a Custom field, and
// the delete of that Tag and the retype of that field check for it under the same lock. A nil
// Condition is the empty group.
func (m *Module) Create(ctx context.Context, ws *ent.Scoped, in segments.CreateInput) (*ent.Segment, error) {
	name, err := validation.Name(in.Name)
	if err != nil {
		return nil, err
	}
	cond := conditiontree.Segment{Group: conditiontree.Empty()}
	if in.Condition != nil {
		cond = *in.Condition
	}
	var seg *ent.Segment
	err = m.bus.WithinWorkspaceLockTx(ctx, ws, func(wl *ent.WorkspaceLocked, _ events.Publisher) error {
		if err := m.segments.Validate(ctx, wl.Scope(), cond); err != nil {
			return err
		}
		seg, err = wl.Segment().Create().SetName(name).SetCondition(cond).Save(ctx)
		return err
	})
	return seg, err
}

// Update validates a changed name and condition exactly as Create does, then applies the change,
// under the Workspace's lock as Create. An archived Segment is segments.ErrArchived: it is
// read-only until it is unarchived (ADR 0100), so the change is an update conditioned on its not
// being archived (ADR 0109) and on the revision it was made on (ADR 0119):
// segments.ErrRevisionChanged when the Segment has moved past it; a missing one is
// segments.ErrNotFound.
func (m *Module) Update(ctx context.Context, ws *ent.Scoped, id int64, in segments.UpdateInput) (*ent.Segment, error) {
	name, err := validation.NamePatch(in.Name)
	if err != nil {
		return nil, err
	}
	in.Name = name
	var seg *ent.Segment
	err = m.bus.WithinWorkspaceLockTx(ctx, ws, func(wl *ent.WorkspaceLocked, _ events.Publisher) error {
		if cond, ok := in.Condition.Get(); ok {
			if err := m.segments.Validate(ctx, wl.Scope(), cond); err != nil {
				return err
			}
		}
		q := wl.Segment().EditAt(id, in.Revision).Where(segment.ArchivedAtIsNil()).
			Refusing(segments.ErrNotFound, segments.ErrRevisionChanged, segments.RefuseArchived)
		in.Name.ApplySet(q.SetName)
		in.Condition.ApplySet(q.SetCondition)
		seg, err = q.Save(ctx)
		return err
	})
	if ent.IsNotFound(err) {
		return nil, segments.ErrNotFound
	}
	return seg, err
}
