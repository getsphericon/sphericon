// Package drafts is the lifecycle every Versioned definition shares (ADR 0065, GLOSSARY:
// Versioned definition): today the Automation and the Rule. The definition lives in versions; the
// object points at its live version, what it does, and at its draft, the one version above live.
// An edit writes the draft and never the live version, Publish makes the draft live and the
// object active, Revert drops the draft, Deactivate stops the object and keeps its live version,
// and Archive stops it before it leaves the lists, so Unarchive never re-arms a send without a
// Publish.
//
// The package owns the moves and their order; each kind of definition supplies its body through
// Definition (merge and validate an edit, compare, write, the check before Publish and before
// Archive), and the generated storage half (ent.VersionedStore, the scoped client's <N>Pointers)
// moves the pointers and the state. Every move runs in one transaction under the row's lock, and
// each write of the row is audited. It takes a *ent.Scoped on every call and holds no client.
package drafts

import (
	"context"
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
)

// Definition is what one kind of Versioned definition supplies to the lifecycle: O is its row, V
// its version, E an edit of its definition and B the body a version stores.
type Definition[O, V, E, B any] interface {
	// Store binds the generated pointer operations to the scope.
	Store(s *ent.Scoped) ent.VersionedStore[O, V]
	// Merge lays the edit over base (nil for a new object) and validates the result. It runs under
	// the row's lock, in the transaction of the move.
	Merge(ctx context.Context, ts *ent.Scoped, base *V, edit E) (B, error)
	// Equal reports whether body is the definition v holds, compared in the canonical form a
	// version stores, so that saving a definition unchanged stores nothing.
	Equal(v *V, body B) bool
	// Write stores body as the object's draft: it rewrites draft in place, or, when draft is nil,
	// inserts the version numbered number. It returns the version's id.
	Write(ctx context.Context, ts *ent.Scoped, id int64, number int, draft *V, body B) (int64, error)
	// Publishable refuses a draft that cannot go live.
	Publishable(ctx context.Context, ts *ent.Scoped, draft *V) error
	// Archivable refuses to archive the object while live work still needs it.
	Archivable(ctx context.Context, ts *ent.Scoped, row *O) error
}

// Errors are the refusals of one kind of definition, in its own words.
type Errors struct {
	// NotFound: no such object in the Workspace, or an archived one where only a kept one acts.
	NotFound error
	// NothingToPublish: the object has neither a draft nor a live version.
	NothingToPublish error
	// NothingToRevert: the object has no draft above a live version.
	NothingToRevert error
}

// Lifecycle runs the moves of one kind of Versioned definition.
type Lifecycle[O, V, E, B any] struct {
	bus  *events.Bus
	def  Definition[O, V, E, B]
	errs Errors
}

// New builds the lifecycle of a kind of definition. The bus opens the transaction of each move.
func New[O, V, E, B any](bus *events.Bus, def Definition[O, V, E, B], errs Errors) *Lifecycle[O, V, E, B] {
	return &Lifecycle[O, V, E, B]{bus: bus, def: def, errs: errs}
}

// errNoVersion: an object with neither a draft nor a live version to edit, which Create never
// leaves.
var errNoVersion = errors.New("drafts: the object has no version to edit")

// Owner is the row of a Versioned definition, as ent generates it.
type Owner[O, V any] interface {
	VersionState() ent.VersionState[O, V]
}

// LiveAndDraft returns the object's live version (what it does; nil until it is first published)
// and its draft (the unpublished changes above live; nil when there are none), as loaded on the
// row.
func LiveAndDraft[O, V any](o Owner[O, V]) (live, draft *V) {
	st := o.VersionState()
	return st.Live, st.Draft
}

// Base is the version an edit starts from: the draft, else the live version.
func Base[O, V any](o Owner[O, V]) *V {
	live, draft := LiveAndDraft(o)
	if draft != nil {
		return draft
	}
	return live
}

// Get returns the object with the id, archived or not, with its live version and draft.
func (l *Lifecycle[O, V, E, B]) Get(ctx context.Context, s *ent.Scoped, id int64) (*O, error) {
	row, err := l.def.Store(s).Load(ctx, id)
	return row, l.notFound(err)
}

// Create stores a new, inactive object whose definition is its draft, version 1: nothing is live
// until it is published. owner writes the object's own row (its name) and returns its id; the
// edit is the whole definition.
func (l *Lifecycle[O, V, E, B]) Create(ctx context.Context, s *ent.Scoped, owner func(ctx context.Context, ts *ent.Scoped) (int64, error), edit E) (*O, error) {
	var id int64
	err := l.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		body, err := l.def.Merge(ctx, ts, nil, edit)
		if err != nil {
			return err
		}
		if id, err = owner(ctx, ts); err != nil {
			return err
		}
		versionID, err := l.def.Write(ctx, ts, id, 1, nil, body)
		if err != nil {
			return err
		}
		return l.def.Store(ts).SetDraft(ctx, id, versionID)
	})
	if err != nil {
		return nil, err
	}
	return l.Get(ctx, s, id)
}

// Edit writes the edit to the object's draft (ADR 0065): the fields it leaves out keep the
// draft's, or the live version's when there is no draft. The live version is never rewritten: the
// first edit above it makes the draft, the version above the newest, and the next ones rewrite the
// draft in place. A definition equal to the one it starts from stores nothing. The status is
// untouched. own, when given, changes the object's own row (its name) under the same lock.
func (l *Lifecycle[O, V, E, B]) Edit(ctx context.Context, s *ent.Scoped, id int64, own func(ctx context.Context, ts *ent.Scoped, row *O) error, edit E) (*O, error) {
	err := l.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		store := l.def.Store(ts)
		st, err := l.lock(ctx, store, id)
		if err != nil {
			return err
		}
		if own != nil {
			if err := own(ctx, ts, st.Row); err != nil {
				return err
			}
		}
		base := st.Draft
		if base == nil {
			base = st.Live
		}
		if base == nil {
			return errNoVersion
		}
		body, err := l.def.Merge(ctx, ts, base, edit)
		if err != nil {
			return err
		}
		if l.def.Equal(base, body) {
			return nil
		}
		if st.Draft != nil {
			_, err := l.def.Write(ctx, ts, id, 0, st.Draft, body)
			return err
		}
		newest, err := store.Newest(ctx, id)
		if err != nil {
			return err
		}
		versionID, err := l.def.Write(ctx, ts, id, newest+1, nil, body)
		if err != nil {
			return err
		}
		return store.SetDraft(ctx, id, versionID)
	})
	if err != nil {
		return nil, err
	}
	return l.Get(ctx, s, id)
}

// Publish makes the draft live and the object active: the old live version stays as history, and
// the work already on it finishes there. With no draft it makes an inactive live version active
// again and leaves an active one as it is; with neither it is refused (NothingToPublish). An object
// moving from inactive to active records the moment (activated_at); one already active keeps its
// own. A draft the definition finds unpublishable is refused and stays. Publish is send-class: the
// surfaces gate it by the publish scope. An archived object acts on nothing and is restored before
// it is published, so it is as good as absent here (NotFound).
func (l *Lifecycle[O, V, E, B]) Publish(ctx context.Context, s *ent.Scoped, id int64) (*O, error) {
	err := l.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		store := l.def.Store(ts)
		st, err := l.lock(ctx, store, id)
		if err != nil {
			return err
		}
		if st.Archived {
			return l.errs.NotFound
		}
		now := time.Now()
		var activatedAt *time.Time
		if !st.Active {
			activatedAt = &now
		}
		switch {
		case st.Draft != nil:
			if err := l.def.Publishable(ctx, ts, st.Draft); err != nil {
				return err
			}
			return store.Promote(ctx, id, *st.DraftID, activatedAt)
		case st.Live == nil:
			return l.errs.NothingToPublish
		case st.Active:
			return nil // live and active already
		default:
			return store.Activate(ctx, id, now)
		}
	})
	if err != nil {
		return nil, err
	}
	return l.Get(ctx, s, id)
}

// Revert drops the draft: the live version is again the latest. It is refused (NothingToRevert)
// unless there is a draft above a live version, since a never-published object's draft is its
// whole definition. Nothing stands on a draft, so the version goes. Revert is authoring.
func (l *Lifecycle[O, V, E, B]) Revert(ctx context.Context, s *ent.Scoped, id int64) (*O, error) {
	err := l.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		store := l.def.Store(ts)
		st, err := l.lock(ctx, store, id)
		if err != nil {
			return err
		}
		if st.LiveID == nil || st.DraftID == nil {
			return l.errs.NothingToRevert
		}
		// The pointer is cleared through the audited write first, so the Audit log records the
		// Revert; the key's SET NULL would leave no trace.
		if err := store.ClearDraft(ctx, id); err != nil {
			return err
		}
		return store.DeleteVersion(ctx, id, *st.DraftID)
	})
	if err != nil {
		return nil, err
	}
	return l.Get(ctx, s, id)
}

// Deactivate makes the object inactive: it stops acting, and the work already in flight finishes.
// Its live version and draft stay. It is authoring: it can only stop sending, never start it. An
// archived object acts on nothing, so it is as good as absent here (NotFound).
func (l *Lifecycle[O, V, E, B]) Deactivate(ctx context.Context, s *ent.Scoped, id int64) (*O, error) {
	err := l.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		store := l.def.Store(ts)
		st, err := l.lock(ctx, store, id)
		if err != nil {
			return err
		}
		if st.Archived {
			return l.errs.NotFound
		}
		if !st.Active {
			return nil
		}
		return store.Deactivate(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	return l.Get(ctx, s, id)
}

// Archive discards the object (ADR 0049): it leaves the lists and acts on nothing, and its history
// stays. It also makes the object inactive, under the same lock as Publish, so an archived object
// is never active and restoring it brings it back inactive. The definition may refuse it while
// live work still needs the object. Archiving an archived object changes nothing.
func (l *Lifecycle[O, V, E, B]) Archive(ctx context.Context, s *ent.Scoped, id int64) error {
	return l.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		store := l.def.Store(ts)
		st, err := l.lock(ctx, store, id)
		if err != nil {
			return err
		}
		if st.Archived {
			return nil
		}
		if err := l.def.Archivable(ctx, ts, st.Row); err != nil {
			return err
		}
		if st.Active {
			if err := store.Deactivate(ctx, id); err != nil {
				return err
			}
		}
		return store.Discard(ctx, id)
	})
}

// Unarchive brings an archived object back to the lists, inactive: only Publish arms it again.
// Restoring one that is not archived changes nothing.
func (l *Lifecycle[O, V, E, B]) Unarchive(ctx context.Context, s *ent.Scoped, id int64) error {
	return l.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		store := l.def.Store(ts)
		st, err := l.lock(ctx, store, id)
		if err != nil {
			return err
		}
		if !st.Archived {
			return nil
		}
		if st.Active {
			if err := store.Deactivate(ctx, id); err != nil {
				return err
			}
		}
		return store.Undiscard(ctx, id)
	})
}

// lock takes the object's row for the rest of the transaction, archived or not, so the moves of
// one object take turns.
func (l *Lifecycle[O, V, E, B]) lock(ctx context.Context, store ent.VersionedStore[O, V], id int64) (ent.VersionState[O, V], error) {
	st, err := store.Lock(ctx, id)
	return st, l.notFound(err)
}

func (l *Lifecycle[O, V, E, B]) notFound(err error) error {
	if ent.IsNotFound(err) {
		return l.errs.NotFound
	}
	return err
}
