package holds

import (
	"context"
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ent"
	enthold "github.com/getsphericon/sphericon/ent/hold"
)

// Input is what a setter states about a hold.
type Input struct {
	ReasonCode string
	Message    string
	// ActionURL is where the Workspace's Owners and Admins can resolve the hold; nil for none.
	ActionURL *string
	ExpiresAt time.Time
}

// Change is the outcome of setting or clearing a hold: the hold concerned (nil when a clear
// found none) and whether the call changed anything.
type Change struct {
	Hold    *ent.Hold
	Changed bool
}

// Set sets the hold under key on the scope's Workspace, or renews it when it exists: one call
// does both and repeating it is harmless (Changed is false when the stored hold already says
// what in says). setBy names who set it.
func Set(ctx context.Context, s *ent.Scoped, key, setBy string, in Input) (Change, error) {
	// A concurrent first set of the same key loses the unique index once; the retry
	// then finds the row and renews it.
	for range 2 {
		existing, err := s.Hold().Query().Where(enthold.Key(key)).Only(ctx)
		switch {
		case ent.IsNotFound(err):
			created, err := s.Hold().Create().
				SetKey(key).SetReasonCode(in.ReasonCode).SetMessage(in.Message).SetNillableActionURL(in.ActionURL).
				SetExpiresAt(in.ExpiresAt).SetSetBy(setBy).
				Save(ctx)
			if ent.IsConstraintError(err) {
				continue
			}
			return Change{Hold: created, Changed: err == nil}, err
		case err != nil:
			return Change{}, err
		}
		if same(existing, in) {
			return Change{Hold: existing}, nil
		}
		update := s.Hold().UpdateOneID(existing.ID).
			SetReasonCode(in.ReasonCode).SetMessage(in.Message).
			SetExpiresAt(in.ExpiresAt).SetSetBy(setBy)
		if in.ActionURL != nil {
			update.SetActionURL(*in.ActionURL)
		} else {
			update.ClearActionURL()
		}
		updated, err := update.Save(ctx)
		return Change{Hold: updated, Changed: err == nil}, err
	}
	return Change{}, errors.New("holds: could not set the hold: it kept changing under us")
}

// Clear removes the hold under key. Clearing a missing hold succeeds and changes nothing.
func Clear(ctx context.Context, s *ent.Scoped, key string) (Change, error) {
	existing, err := s.Hold().Query().Where(enthold.Key(key)).Only(ctx)
	if ent.IsNotFound(err) {
		return Change{}, nil
	}
	if err != nil {
		return Change{}, err
	}
	err = s.Hold().DeleteOneID(existing.ID).Exec(ctx)
	if ent.IsNotFound(err) {
		return Change{}, nil
	}
	return Change{Hold: existing, Changed: err == nil}, err
}

// same reports whether the stored hold already says what in says. Timestamps are compared at
// the database's microsecond precision.
func same(h *ent.Hold, in Input) bool {
	return h.ReasonCode == in.ReasonCode && h.Message == in.Message && equalStrings(h.ActionURL, in.ActionURL) &&
		h.ExpiresAt.Truncate(time.Microsecond).Equal(in.ExpiresAt.Truncate(time.Microsecond))
}

func equalStrings(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
