// Package suppression is the one place Suppression rows are written, listed and removed
// (ADR 0015, GLOSSARY "Suppression"): the hard, global do-not-send registry keyed by
// (channel, destination). The bus subscriber, the manual add and the site handlers all
// go through it, so the "insert, ignore on conflict" statement exists once. Unsubscribe
// and confirm stay in consent; eligibility reads Suppression with its own SQL.
package suppression

import (
	"context"
	"errors"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/pagination"
)

var (
	// ErrDestinationEmpty: the destination is blank after normalization.
	ErrDestinationEmpty = errors.New("suppression: destination must not be empty")
	// ErrNotFound: no such Suppression in this Workspace.
	ErrNotFound = errors.New("suppression: not found")
)

// NormalizeDestination is the single definition of "normalized destination":
// lower-cased and trimmed. Suppression and Unsubscribe rows store destinations this
// way, and every lookup must apply it so the readers of those rows agree on one form.
func NormalizeDestination(dest string) string {
	return strings.ToLower(strings.TrimSpace(dest))
}

// Bounce kinds. Only a permanent (hard) bounce suppresses; a transient (soft) one, such
// as a full mailbox, is temporary.
const (
	BounceKindPermanent = "permanent"
	BounceKindTransient = "transient"
)

// Names of the delivery events that can suppress; the bus's event names are these.
const (
	EventBounced    = "email.bounced"
	EventComplained = "email.complained"
)

// ForEvent is the rule that decides whether a delivery event suppresses its address, and
// why: a complaint always does, a bounce only when permanent (bounceKind), anything else
// never. Unsubscribe is not a Suppression (ADR 0001).
func ForEvent(name, bounceKind string) (entsuppression.Reason, bool) {
	switch name {
	case EventComplained:
		return entsuppression.ReasonComplaint, true
	case EventBounced:
		if bounceKind == BounceKindPermanent {
			return entsuppression.ReasonBounce, true
		}
		return "", false
	default:
		return "", false
	}
}

// Entry is one Suppression to add. ContactID is the Contact the destination belongs
// to, when known (display only); zero means none.
type Entry struct {
	Destination string
	Reason      entsuppression.Reason
	ContactID   int64
}

// Add suppresses an email destination in the scoped Workspace with one insert. It is
// idempotent per (channel, destination): an existing entry keeps its reason.
func Add(ctx context.Context, s *ent.Scoped, e Entry) error {
	dest := NormalizeDestination(e.Destination)
	if dest == "" {
		return ErrDestinationEmpty
	}
	create := s.Suppression().Create().
		SetChannel(entsuppression.ChannelEmail).
		SetDestination(dest).
		SetReason(e.Reason)
	if e.ContactID != 0 {
		create.SetContactID(e.ContactID)
	}
	return create.
		OnConflictColumns(entsuppression.FieldWorkspaceID, entsuppression.FieldChannel, entsuppression.FieldDestination).
		Ignore().
		Exec(ctx)
}

// AddManual is Add with the manual reason (an Owner's or an API caller's request), and
// returns the entry, the existing one when the destination was already suppressed.
func AddManual(ctx context.Context, s *ent.Scoped, destination string) (*ent.Suppression, error) {
	if err := Add(ctx, s, Entry{Destination: destination, Reason: entsuppression.ReasonManual}); err != nil {
		return nil, err
	}
	return s.Suppression().Query().
		Where(entsuppression.ChannelEQ(entsuppression.ChannelEmail), entsuppression.DestinationEQ(NormalizeDestination(destination))).
		Only(ctx)
}

// List returns one page of the Workspace's Suppressions, newest first.
func List(ctx context.Context, s *ent.Scoped, p pagination.Params) (pagination.Page[*ent.Suppression], error) {
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return s.Suppression().Query().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Suppression, error) {
			return s.Suppression().Query().
				Order(ent.Desc(entsuppression.FieldID)).
				Limit(limit).
				Offset(offset).
				All(ctx)
		})
}

// Remove deletes a Suppression of the scoped Workspace by id; a missing one is ErrNotFound.
func Remove(ctx context.Context, s *ent.Scoped, id int64) error {
	err := s.Suppression().DeleteOneID(id).Exec(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

// DetachContact clears the Contact reference of every Suppression naming it, for the
// Contact's Erasure: the rows are keyed by destination and keep refusing the address.
func DetachContact(ctx context.Context, s *ent.Scoped, contactID int64) error {
	_, err := s.Suppression().Update().Where(entsuppression.ContactID(contactID)).ClearContactID().Save(ctx)
	return err
}
