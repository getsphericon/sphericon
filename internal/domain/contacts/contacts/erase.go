package contacts

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// DeleteIn removes the Contact row inside the transaction s is bound to; its Tag links and Custom
// field values go with the row. It is the unchecked removal (ADR 0102): deleting a Contact is
// Erasure (ADR 0021), and the row alone would leave the person's Events, Visitors, delivery records
// and queued work behind. Erasure is the use case internal/usecase/contacterasure, which erases
// those through their owners and removes the row last, in one transaction; easement forbids every
// other package to call DeleteIn. contactID is 0 for a subject without a Contact, which removes
// nothing.
func DeleteIn(ctx context.Context, s *ent.Scoped, contactID int64) error {
	if contactID == 0 {
		return nil
	}
	return s.Contact().DeleteOneID(contactID).Exec(ctx)
}
