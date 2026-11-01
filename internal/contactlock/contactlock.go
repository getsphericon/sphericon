// Package contactlock is the one row lock that orders the writes about one Contact: an Enrollment
// made or ended (#751), a change of its Tags or of its Custom fields recorded as a System event
// (#897). Each takes it first and holds it to the end of its transaction, so two of them run one
// after the other and each reads what the other committed.
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
