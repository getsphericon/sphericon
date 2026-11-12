package versioneddefinition

import (
	"context"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
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

// Change supplies an operation and, for Draft, the version to attach as the new draft.
type Change struct {
	Trigger Trigger
	DraftID int64
}

// Machine configures the activity transitions for one Change against a loaded snapshot. Fire
// change.Trigger once, inside the transaction that loaded before and owns the remaining effects.
// Versions and deletion markers are guards and transition data, not additional machine states.
// Both a status transition and an internal action use the same conditional position writer.
func Machine(before ent.VersionPosition, change Change, write func(context.Context, ent.VersionPosition, ent.VersionPosition) (bool, error)) *stateless.StateMachine {
	const inactive, active = false, true
	current := before.Active
	after := before
	persist := func(ctx context.Context, to bool) (bool, error) {
		next := after
		next.Active = to
		swapped, err := write(ctx, before, next)
		if err == nil && swapped {
			current = to
		}
		return swapped, err
	}
	m := lifecycle.New(lifecycle.Storage[bool]{
		Read: func(context.Context) (bool, error) { return current, nil },
		Write: func(ctx context.Context, _, to bool) (bool, error) {
			return persist(ctx, to)
		},
	})
	internal := func(ctx context.Context, _ ...any) error {
		// stateless does not call the external storage writer for an InternalTransition.
		swapped, err := persist(ctx, before.Active)
		if err != nil {
			return err
		}
		if !swapped {
			return lifecycle.ErrInvalidTransition
		}
		return nil
	}
	when := func(allowed bool) stateless.GuardFunc {
		return func(context.Context, ...any) bool { return allowed }
	}
	kept := when(!before.Archived && !before.Discarded)
	hasDefinition := when(before.LiveID != 0 || before.DraftID != 0)
	off, on := m.Configure(inactive), m.Configure(active)
	switch change.Trigger {
	case Draft:
		after.DraftID = change.DraftID
		for _, cfg := range []*stateless.StateConfiguration{off, on} {
			cfg.InternalTransition(Draft, internal, kept, when(change.DraftID > 0))
		}
	case Edit:
		for _, cfg := range []*stateless.StateConfiguration{off, on} {
			cfg.InternalTransition(Edit, internal, kept, hasDefinition)
		}
	case Revert:
		after.DraftID = 0
		for _, cfg := range []*stateless.StateConfiguration{off, on} {
			cfg.InternalTransition(Revert, internal, kept, when(before.LiveID != 0 && before.DraftID != 0))
		}
	case Publish:
		if before.DraftID != 0 {
			after.LiveID = before.DraftID
			after.DraftID = 0
		}
		off.Permit(Publish, active, kept, hasDefinition)
		on.InternalTransition(Publish, internal, kept, when(before.DraftID != 0))
		on.Ignore(Publish, kept, when(before.LiveID != 0 && before.DraftID == 0))
	case Deactivate:
		on.Permit(Deactivate, inactive, kept, hasDefinition)
		off.Ignore(Deactivate, kept, hasDefinition)
	case Archive:
		after.Archived = true
		on.Permit(Archive, inactive, kept, hasDefinition)
		off.InternalTransition(Archive, internal, kept, hasDefinition)
		off.Ignore(Archive, when(before.Archived && !before.Discarded))
	case Unarchive:
		after.Archived = false
		off.InternalTransition(Unarchive, internal, when(before.Archived && !before.Discarded), hasDefinition)
		for _, cfg := range []*stateless.StateConfiguration{off, on} {
			cfg.Ignore(Unarchive, kept, hasDefinition)
		}
	case Discard:
		after.Discarded = true
		on.Permit(Discard, inactive, kept, hasDefinition)
		off.InternalTransition(Discard, internal, kept, hasDefinition)
		off.Ignore(Discard, when(before.Discarded && !before.Archived))
	}
	return m
}

// move updates the owner before a body edit: the transaction holds its update lock until the
// draft's write or removal commits. The generated writer also compares the loaded revision, and
// writes rename (empty keeps the name) with the position.
func (l *Lifecycle[O, V, E, B]) move(ctx context.Context, store ent.VersionedStore[O, V], before ent.VersionState[O, V], event Trigger, draftID int64, rename string) error {
	m := Machine(before.VersionPosition, Change{Trigger: event, DraftID: draftID},
		func(ctx context.Context, _, after ent.VersionPosition) (bool, error) {
			after.Rename = rename
			return store.CompareAndSwap(ctx, before, after, l.now())
		})
	return m.FireCtx(ctx, event)
}
