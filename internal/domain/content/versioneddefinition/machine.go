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

type trigger string

const (
	triggerDraft      trigger = "draft"
	triggerEdit       trigger = "edit"
	triggerPublish    trigger = "publish"
	triggerRevert     trigger = "revert"
	triggerDeactivate trigger = "deactivate"
	triggerArchive    trigger = "archive"
	triggerUnarchive  trigger = "unarchive"
	triggerDiscard    trigger = "discard"
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
	m.Configure(state(0)).Permit(triggerDraft, hasDraft)
	for _, st := range []state{hasDraft, hasLive, hasLive | hasDraft, active | hasLive, active | hasLive | hasDraft} {
		cfg := m.Configure(st).PermitReentry(triggerEdit)
		if st&hasDraft != 0 {
			cfg.PermitReentry(triggerDraft)
		} else {
			cfg.Permit(triggerDraft, st|hasDraft)
		}
		if st == active|hasLive {
			cfg.Ignore(triggerPublish)
		} else {
			cfg.Permit(triggerPublish, active|hasLive)
		}
		if st&hasLive != 0 && st&hasDraft != 0 {
			cfg.Permit(triggerRevert, st&^hasDraft)
		}
		if st&active != 0 {
			cfg.Permit(triggerDeactivate, st&^active)
		} else {
			cfg.Ignore(triggerDeactivate)
		}
		cfg.Permit(triggerArchive, st&^active|archived)
		cfg.Permit(triggerDiscard, st&^active|discarded)
		cfg.Ignore(triggerUnarchive)
	}
	for _, st := range []state{hasDraft, hasLive, hasLive | hasDraft} {
		m.Configure(st|archived).Permit(triggerUnarchive, st).Ignore(triggerArchive)
		m.Configure(st | discarded).Ignore(triggerDiscard)
	}
	return m
}

// move holds all state writes behind the transition table. Updating the owner first also fences
// a body edit: the transaction holds its update lock until the draft's write or removal commits.
func (l *Lifecycle[O, V, E, B]) move(ctx context.Context, store ent.VersionedStore[O, V], before ent.VersionState[O, V], event trigger, draftID int64) error {
	m := machine(lifecycle.Storage[state]{
		Read: func(context.Context) (state, error) { return positionState(before.VersionPosition), nil },
		Write: func(ctx context.Context, _ state, to state) (bool, error) {
			after := before.VersionPosition
			after.Active = to&active != 0
			after.Archived = to&archived != 0
			after.Discarded = to&discarded != 0
			switch event {
			case triggerDraft:
				after.DraftID = draftID
			case triggerPublish:
				if before.DraftID != 0 {
					after.LiveID = before.DraftID
					after.DraftID = 0
				}
			case triggerRevert:
				after.DraftID = 0
			case triggerEdit, triggerDeactivate, triggerArchive, triggerUnarchive, triggerDiscard:
			}
			return store.CompareAndSwap(ctx, before, after, l.now())
		},
	})
	return m.FireCtx(ctx, event)
}
