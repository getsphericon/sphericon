// Package contactlock is the one row lock that orders the writes about one Contact: an Enrollment
// made or ended (#751), a change of its Tags or of its Custom fields recorded as a System event
// (#897), its Erasure. Each takes it first and holds it to the end of its transaction, so two of
// them run one after the other and each reads what the other committed.
//
// First means before any other row about the Contact, an Enrollment's above all (#811): a
// transaction that writes an Enrollment and then the Contact (a tag step, an Erasure) and one that
// locks the Contact and then ends the Enrollment (an exit Event) would otherwise wait for each
// other, and Postgres would abort one of them.
package contactlock

import (
	"context"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
)

// Lock reads the Contact under FOR NO KEY UPDATE, to the end of the transaction s is bound to. Not
// FOR UPDATE: a row that references the Contact (an Event stored for it, the Outbound message of a
// step's send) takes the key-share lock, which this one leaves free, so neither an Events consumer
// nor a send in flight ever waits for it. The update of the Contact that follows takes the same
// lock anyway. An ent not-found error when the Contact is not in the Workspace.
func Lock(ctx context.Context, s *ent.Scoped, contactID int64) (*ent.Contact, error) {
	q := s.Contact().Query().Where(contact.ID(contactID))
	q.Modify(func(sel *entsql.Selector) { sel.For(entsql.LockNoKeyUpdate) })
	return q.Only(ctx)
}

// Referenced is contactID while the Contact is in the Workspace, read under FOR KEY SHARE, the lock
// the foreign key of a row that references it takes, so the Contact cannot go before that row
// commits (ADR 0106); 0 when it is gone already (erased between a send and the click on its link).
// A row keyed by its Destination is then written unlinked, rather than violating the foreign key.
// contactID 0 is 0.
func Referenced(ctx context.Context, s *ent.Scoped, contactID int64) (int64, error) {
	if contactID == 0 {
		return 0, nil
	}
	q := s.Contact().Query().Where(contact.ID(contactID))
	q.Modify(func(sel *entsql.Selector) { sel.For(entsql.LockKeyShare) })
	ids, err := q.IDs(ctx)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	return contactID, nil
}
