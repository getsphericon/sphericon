package holds

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/getsphericon/sphericon/ent"
	enthold "github.com/getsphericon/sphericon/ent/hold"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The unique constraint of the Holds (ADR 0106): no client can break it.
var _ = db.Declare(
	db.Fault("holds_workspace_id_key", "a hold is written with ON CONFLICT DO NOTHING, and a first set that finds one renews it"),
)

// Input is what a setter states about a hold.
type Input struct {
	ReasonCode string
	Message    string
	// ActionURL is where the Workspace's Owners and Admins can resolve the hold; nil for none.
	ActionURL *string
	ExpiresAt time.Time
}

// DefaultMaxExpiry is how far ahead a hold's expiry may be set when the Instance does not say
// otherwise: a service that stops renewing frees the Workspace by then.
const DefaultMaxExpiry = 30 * 24 * time.Hour

// Limits are what a hold's expiry is checked against: the moment of the set, and how far ahead of
// it the Instance lets an expiry be (SPHERICON_STAFF_HOLD_MAX_EXPIRY).
type Limits struct {
	Now       time.Time
	MaxExpiry time.Duration
}

var (
	// ErrExpiryInPast refuses a hold that would already have lapsed.
	ErrExpiryInPast = validation.RefuseField(fieldname.ExpiresAt, "holds: a hold's expiry must be in the future",
		i18n.ErrorsHoldExpiryInPast)
	// ErrTooLong refuses a hold whose expiry is beyond the Instance's maximum; the refusal names
	// the maximum.
	ErrTooLong = validation.RefuseField(fieldname.ExpiresAt, "holds: a hold's expiry is too far ahead", i18n.ErrorsHoldTooLong)
	// ErrActionURL refuses an action URL that is not an absolute https URL: it is shown to the
	// Workspace's Owners as a link, so no other scheme and no credentials in it.
	ErrActionURL = validation.RefuseField(fieldname.ActionURL, "holds: a hold's action URL must be an absolute https URL",
		i18n.ErrorsHoldActionUrlInvalid)
)

// check refuses an input the rules of a hold refuse: the contract's bounds of the reason code, the
// message and the action URL (package bounds), an action URL that is no absolute https URL without
// credentials, and an expiry that is not ahead of now or further than the maximum.
func (in Input) check(lim Limits) error {
	if err := validation.Length(fieldname.ReasonCode, in.ReasonCode,
		bounds.HoldReasonCodeMinLength, bounds.HoldReasonCodeMaxLength); err != nil {
		return err
	}
	if err := validation.Pattern(fieldname.ReasonCode, in.ReasonCode, bounds.HoldReasonCodePattern); err != nil {
		return err
	}
	if err := validation.Length(fieldname.Message, in.Message, bounds.HoldMessageMinLength, bounds.HoldMessageMaxLength); err != nil {
		return err
	}
	if in.ActionURL != nil {
		if err := validation.MaxLength(fieldname.ActionURL, *in.ActionURL, bounds.HoldActionURLMaxLength); err != nil {
			return err
		}
		if err := validation.Pattern(fieldname.ActionURL, *in.ActionURL, bounds.HoldActionURLPattern); err != nil {
			return err
		}
		if u, err := url.Parse(*in.ActionURL); err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return ErrActionURL
		}
	}
	if !in.ExpiresAt.After(lim.Now) {
		return ErrExpiryInPast
	}
	if in.ExpiresAt.After(lim.Now.Add(lim.MaxExpiry)) {
		return ErrTooLong.With("at most " + lim.MaxExpiry.String() + " from now")
	}
	return nil
}

// Change is the outcome of setting or clearing a hold: the hold concerned (nil when a clear
// found none) and whether the call changed anything.
type Change struct {
	Hold    *ent.Hold
	Changed bool
}

// Set sets the hold under key on the scope's Workspace, or renews it when it exists: one call
// does both and repeating it is harmless (Changed is false when the stored hold already says
// what in says). setBy names who set it. The input is refused when the rules of a hold refuse it
// (its bounds, its action URL, its expiry against lim); nothing is set then.
func Set(ctx context.Context, s *ent.Scoped, key, setBy string, in Input, lim Limits) (Change, error) {
	if err := in.check(lim); err != nil {
		return Change{}, err
	}
	// A concurrent first set of the same key inserts nothing; the retry then finds the row
	// and renews it.
	for range 2 {
		existing, err := s.Hold().Query().Where(enthold.Key(key)).Only(ctx)
		switch {
		case ent.IsNotFound(err):
			id, err := s.Hold().Create().
				SetKey(key).SetReasonCode(in.ReasonCode).SetMessage(in.Message).SetNillableActionURL(in.ActionURL).
				SetExpiresAt(in.ExpiresAt).SetSetBy(setBy).
				OnConflictColumns(enthold.FieldWorkspaceID, enthold.FieldKey).DoNothing().
				ID(ctx)
			inserted, err := db.Inserted(err)
			if err != nil {
				return Change{}, err
			}
			if !inserted {
				continue
			}
			created, err := s.Hold().Get(ctx, id)
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
