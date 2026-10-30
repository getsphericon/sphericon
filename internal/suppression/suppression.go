// Package suppression is the one place Suppression rows are written, listed and removed
// (ADR 0015, GLOSSARY "Suppression"): the hard, global do-not-send registry keyed by
// (channel, destination). The bus subscriber, the manual add and the site handlers all
// go through it, so the "insert, ignore on conflict" statement exists once. Unsubscribe
// and confirm stay in consent; eligibility reads Suppression with its own SQL.
package suppression

import (
	"context"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/validation"
)

var (
	// ErrDestinationEmpty: the destination is blank after normalization. Every surface answers
	// it alike, on the `destination` field (review rule 15).
	ErrDestinationEmpty = validation.RefuseField("destination", "suppression: destination must not be empty", "errors.destination_invalid")
	// ErrNotFound: no such Suppression in this Workspace.
	ErrNotFound = validation.NotFound("suppression: not found", "errors.suppression_not_found")
)

// NormalizeDestination is the single definition of "normalized destination":
// lower-cased and trimmed. Suppression and Unsubscribe rows store destinations this
// way, and every lookup must apply it so the readers of those rows agree on one form.
func NormalizeDestination(dest string) string {
	return strings.ToLower(strings.TrimSpace(dest))
}

// Bounce kinds. Only a hard bounce suppresses; a soft one, such as a full mailbox, is
// temporary.
const (
	BounceKindHard = "hard"
	BounceKindSoft = "soft"
)

// Names of the delivery events that can suppress; the bus's event names are these.
const (
	EventBounced    = "email.bounced"
	EventComplained = "email.complained"
)

// ForEvent is the rule that decides whether a delivery event suppresses its address, and
// why: a complaint always does, a bounce only when hard (bounceKind), anything else
// never. Unsubscribe is not a Suppression (ADR 0001).
func ForEvent(name, bounceKind string) (entsuppression.Reason, bool) {
	switch name {
	case EventComplained:
		return entsuppression.ReasonComplaint, true
	case EventBounced:
		if bounceKind == BounceKindHard {
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
// idempotent per (channel, destination): an existing active entry keeps its reason. A
// lifted entry does not count (ADR 0049): the destination is suppressed anew in a new row.
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
		OnConflictColumnsWhere(sql.IsNull(entsuppression.FieldLiftedAt),
			entsuppression.FieldWorkspaceID, entsuppression.FieldChannel, entsuppression.FieldDestination).
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
		Where(entsuppression.ChannelEQ(entsuppression.ChannelEmail), entsuppression.DestinationEQ(NormalizeDestination(destination)), entsuppression.LiftedAtIsNil()).
		Only(ctx)
}

// List returns one page of the Workspace's active Suppressions, newest first, or of its
// lifted ones when lifted is true.
func List(ctx context.Context, s *ent.Scoped, p pagination.Params, lifted bool) (pagination.Page[*ent.Suppression], error) {
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return query(s, lifted).Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Suppression, error) {
			return query(s, lifted).
				Order(ent.Desc(entsuppression.FieldID)).
				Limit(limit).
				Offset(offset).
				All(ctx)
		})
}

// query is the Workspace's active Suppressions, or its lifted ones.
func query(s *ent.Scoped, lifted bool) *ent.SuppressionQuery {
	if lifted {
		return s.Suppression().Query().Where(entsuppression.LiftedAtNotNil())
	}
	return s.Suppression().Query().Where(entsuppression.LiftedAtIsNil())
}

// Remove lifts an active Suppression of the scoped Workspace by id: the row stays with its
// lift time and the destination can receive mail again. A missing or already lifted one is
// ErrNotFound.
func Remove(ctx context.Context, s *ent.Scoped, id int64) error {
	n, err := s.Suppression().Update().
		Where(entsuppression.ID(id), entsuppression.LiftedAtIsNil()).
		SetLiftedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// EraseContact is the Suppression part of the Contact's Erasure: its lifted rows are
// deleted, since a lifted one refuses nothing any more, and its active rows only lose the
// Contact reference, because they are keyed by destination and keep refusing the address.
func EraseContact(ctx context.Context, s *ent.Scoped, contactID int64) error {
	if _, err := s.Suppression().Delete().
		Where(entsuppression.ContactID(contactID), entsuppression.LiftedAtNotNil()).
		Exec(ctx); err != nil {
		return err
	}
	_, err := s.Suppression().Update().Where(entsuppression.ContactID(contactID)).ClearContactID().Save(ctx)
	return err
}
