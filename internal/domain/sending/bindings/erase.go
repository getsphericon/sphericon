package bindings

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/starttoken"
	"github.com/getsphericon/sphericon/ent/visitor"
)

// EraseSubject is the Telegram part of a Contact's Erasure (ADR 0021): it deletes the Bindings and
// Start tokens of the Contact and of its Visitors (a Visitor's are owned by the Visitor row, which
// the Erasure deletes later), so it runs before the Visitors go. contactID is 0 for an anonymous
// Visitor without a Contact; visitorIDs are the subject's visitor ids. Telegram opt-outs are not
// here: they are keyed by the user id and stay. Run it inside the Erasure's transaction.
func EraseSubject(ctx context.Context, s *ent.Scoped, contactID int64, visitorIDs []string) error {
	var owners []predicate.Visitor
	if contactID != 0 {
		owners = append(owners, visitor.ContactID(contactID))
	}
	if len(visitorIDs) > 0 {
		owners = append(owners, visitor.VisitorIDIn(visitorIDs...))
	}
	var bindings []predicate.Binding
	var tokens []predicate.StartToken
	if contactID != 0 {
		bindings = append(bindings, binding.ContactID(contactID))
		tokens = append(tokens, starttoken.ContactID(contactID))
	}
	if len(owners) > 0 {
		ids, err := s.Visitor().Query().Where(visitor.Or(owners...)).IDs(ctx)
		if err != nil {
			return err
		}
		if len(ids) > 0 {
			bindings = append(bindings, binding.VisitorIDIn(ids...))
			tokens = append(tokens, starttoken.VisitorIDIn(ids...))
		}
	}
	if len(bindings) == 0 {
		return nil
	}
	if _, err := s.Binding().Delete().Where(binding.Or(bindings...)).Exec(ctx); err != nil {
		return err
	}
	_, err := s.StartToken().Delete().Where(starttoken.Or(tokens...)).Exec(ctx)
	return err
}
