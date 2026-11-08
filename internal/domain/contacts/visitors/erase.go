package visitors

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/visitor"
)

// EraseSubject is the Visitor part of a Contact's Erasure (ADR 0021): it deletes the Visitors bound
// to the Contact and those of the subject's visitor ids, and their Telegram Bindings and Start
// tokens go with them by their cascading foreign keys. contactID is 0 for an anonymous Visitor
// without a Contact. Run it inside the Erasure's transaction, after the Visitors' Events are gone.
func EraseSubject(ctx context.Context, s *ent.Scoped, contactID int64, visitorIDs []string) error {
	match := visitor.VisitorIDIn(visitorIDs...)
	if contactID != 0 {
		match = visitor.Or(visitor.ContactID(contactID), match)
	}
	_, err := s.Visitor().Delete().Where(match).Exec(ctx)
	return err
}
