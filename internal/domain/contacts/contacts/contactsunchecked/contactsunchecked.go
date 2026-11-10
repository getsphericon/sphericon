// Package contactsunchecked holds the contacts module's unchecked operations (ADR 0110): the retype
// of a Custom field, whose check reads the Conditions of the modules above contacts, and the removal
// of the Contact row, which only Erasure makes after it has erased the person's rows of the other
// owners. Their checks are the use cases' (internal/usecase/customfieldretyping and
// internal/usecase/contacterasure), the one kind of package easement lets import this one. The
// contacts module does not import it.
package contactsunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// UpdateCustomFieldIn renames field or changes its type, an audited change (ADR 0022). A type
// change rewrites no stored value: the leaves of the new type read the values as they are, and are
// false on one of another shape. It does not ask whether a Segment, a Rule or a Broadcast tests the
// field with a leaf the new type does not take: internal/usecase/customfieldretyping runs the
// Removal check first. A new name is held to the name rule (validation.Name).
func UpdateCustomFieldIn(ctx context.Context, s *ent.Scoped, field *ent.CustomField, in contacts.CustomFieldUpdateInput) (*ent.CustomField, error) {
	name, err := validation.NamePatch(in.Name)
	if err != nil {
		return nil, err
	}
	upd := s.CustomField().UpdateOneID(field.ID)
	patch.ApplySet(name, upd.SetName)
	if t, ok := in.Retype(field); ok {
		upd.SetType(t)
	}
	return upd.Save(ctx)
}

// DeleteIn removes the Contact row inside the transaction s is bound to; its Tag links and Custom
// field values go with the row. Deleting a Contact is Erasure (ADR 0021), and the row alone would
// leave the person's Events, Visitors, delivery records and queued work behind:
// internal/usecase/contacterasure erases those through their owners and removes the row last, in
// one transaction. contactID is 0 for a subject without a Contact, which removes nothing.
func DeleteIn(ctx context.Context, s *ent.Scoped, contactID int64) error {
	if contactID == 0 {
		return nil
	}
	return s.Contact().DeleteOneID(contactID).Exec(ctx)
}
