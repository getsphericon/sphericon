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
	"github.com/getsphericon/sphericon/ent/suppression"
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
// way, and every lookup must apply it so the point-lookup path (eligibility.Check) and
// the batch SQL path (Predicate, which folds case via lower()) agree.
func NormalizeDestination(dest string) string {
	return strings.ToLower(strings.TrimSpace(dest))
}

// BounceKindPermanent is the kind of a hard bounce, the only one that suppresses.
const BounceKindPermanent = "permanent"

// ForBounce is the rule that decides a bounce is permanent: only a hard bounce
// suppresses, with the bounce reason; a soft bounce (a full mailbox) never does.
func ForBounce(kind string) (suppression.Reason, bool) {
	if kind == BounceKindPermanent {
		return suppression.ReasonBounce, true
	}
	return "", false
}

// Entry is one Suppression to add. ContactID is the Contact the destination belongs
// to, when known (display only); zero means none.
type Entry struct {
	Destination string
	Reason      suppression.Reason
	ContactID   int64
}

// Add suppresses an email destination in the scoped Workspace. It is idempotent per
// (channel, destination): an existing entry, with its reason, is returned unchanged.
func Add(ctx context.Context, s *ent.Scoped, e Entry) (*ent.Suppression, error) {
	dest := NormalizeDestination(e.Destination)
	if dest == "" {
		return nil, ErrDestinationEmpty
	}
	create := s.Suppression().Create().
		SetChannel(suppression.ChannelEmail).
		SetDestination(dest).
		SetReason(e.Reason)
	if e.ContactID != 0 {
		create.SetContactID(e.ContactID)
	}
	if err := create.
		OnConflictColumns(suppression.FieldWorkspaceID, suppression.FieldChannel, suppression.FieldDestination).
		Ignore().
		Exec(ctx); err != nil {
		return nil, err
	}
	return s.Suppression().Query().
		Where(suppression.ChannelEQ(suppression.ChannelEmail), suppression.DestinationEQ(dest)).
		Only(ctx)
}

// List returns one page of the Workspace's Suppressions, newest first.
func List(ctx context.Context, s *ent.Scoped, p pagination.Params) (pagination.Page[*ent.Suppression], error) {
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return s.Suppression().Query().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Suppression, error) {
			return s.Suppression().Query().
				Order(ent.Desc(suppression.FieldID)).
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
	_, err := s.Suppression().Update().Where(suppression.ContactID(contactID)).ClearContactID().Save(ctx)
	return err
}
