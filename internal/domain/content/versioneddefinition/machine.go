package versioneddefinition

import (
	"context"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// state is the shape of a position: version ids belong to the conditional write, while the
// presence of live and draft versions determines which moves the lifecycle permits.
type state uint8

const (
	hasLive state = 1 << iota
	hasDraft
	active
	archived
	discarded
)

// Trigger names one operation of the Versioned definition lifecycle.
type Trigger string

const (
	Draft      Trigger = "draft"
	Edit       Trigger = "edit"
	Publish    Trigger = "publish"
	Revert     Trigger = "revert"
	Deactivate Trigger = "deactivate"
	Archive    Trigger = "archive"
	Unarchive  Trigger = "unarchive"
	Discard    Trigger = "discard"
)

func positionState(p ent.VersionPosition) state {
	var st state
	if p.LiveID != 0 {
		st |= hasLive
	}
	if p.DraftID != 0 {
		st |= hasDraft
	}
	if p.Active {
		st |= active
	}
	if p.Archived {
		st |= archived
	}
	if p.Discarded {
		st |= discarded
	}
	return st
}

// machine declares every move of a Versioned definition, including changes that keep its
// activity but move a pointer. The empty state exists only inside Create's transaction.
func machine(storage lifecycle.Storage[state]) *stateless.StateMachine {
	m := lifecycle.New(storage)
	m.Configure(state(0)).Permit(Draft, hasDraft)
	for _, st := range []state{hasDraft, hasLive, hasLive | hasDraft, active | hasLive, active | hasLive | hasDraft} {
		cfg := m.Configure(st).PermitReentry(Edit)
		if st&hasDraft != 0 {
			cfg.PermitReentry(Draft)
		} else {
			cfg.Permit(Draft, st|hasDraft)
		}
		if st == active|hasLive {
			cfg.Ignore(Publish)
		} else {
			cfg.Permit(Publish, active|hasLive)
		}
		if st&hasLive != 0 && st&hasDraft != 0 {
			cfg.Permit(Revert, st&^hasDraft)
		}
		if st&active != 0 {
			cfg.Permit(Deactivate, st&^active)
		} else {
			cfg.Ignore(Deactivate)
		}
		cfg.Permit(Archive, st&^active|archived)
		cfg.Permit(Discard, st&^active|discarded)
		cfg.Ignore(Unarchive)
	}
	for _, st := range []state{hasDraft, hasLive, hasLive | hasDraft} {
		m.Configure(st|archived).Permit(Unarchive, st).Ignore(Archive)
		m.Configure(st | discarded).Ignore(Discard)
	}
	return m
}

// Change supplies an operation and, for Draft, the version to attach as the new draft.
type Change struct {
	Trigger Trigger
	DraftID int64
}

// Transition runs the table and moves the full position through storage's conditional writer.
// The caller keeps its effects in the same transaction after this move. A Draft needs a positive
// version id; all other pointer changes are determined by the position, not by the caller.
func Transition(ctx context.Context, storage lifecycle.Storage[ent.VersionPosition], change Change) error {
	if change.Trigger == Draft && change.DraftID <= 0 {
		return lifecycle.ErrInvalidTransition
	}
	var before ent.VersionPosition
	m := machine(lifecycle.Storage[state]{
		Read: func(ctx context.Context) (state, error) {
			var err error
			before, err = storage.Read(ctx)
			return positionState(before), err
		},
		Write: func(ctx context.Context, _ state, to state) (bool, error) {
			after := before
			after.Active = to&active != 0
			after.Archived = to&archived != 0
			after.Discarded = to&discarded != 0
			switch change.Trigger {
			case Draft:
				after.DraftID = change.DraftID
			case Publish:
				if before.DraftID != 0 {
					after.LiveID = before.DraftID
					after.DraftID = 0
				}
			case Revert:
				after.DraftID = 0
			case Edit, Deactivate, Archive, Unarchive, Discard:
			}
			return storage.Write(ctx, before, after)
		},
	})
	return m.FireCtx(ctx, change.Trigger)
}

// move updates the owner before a body edit: the transaction holds its update lock until the
// draft's write or removal commits. The generated writer also compares the loaded revision.
func (l *Lifecycle[O, V, E, B]) move(ctx context.Context, store ent.VersionedStore[O, V], before ent.VersionState[O, V], event Trigger, draftID int64) error {
	return Transition(ctx, lifecycle.Storage[ent.VersionPosition]{
		Read: func(context.Context) (ent.VersionPosition, error) { return before.VersionPosition, nil },
		Write: func(ctx context.Context, _, after ent.VersionPosition) (bool, error) {
			return store.CompareAndSwap(ctx, before, after, l.now())
		},
	}, Change{Trigger: event, DraftID: draftID})
}
