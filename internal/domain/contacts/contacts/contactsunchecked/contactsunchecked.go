// Package contactsunchecked holds the contacts module's unchecked operations (ADR 0110): the edit
// of a Custom field, whose retype's check reads the Conditions of the modules above contacts and
// which keeps every rule of the field's own row (ADR 0145), and the removal
// of the Contact row, which only Erasure makes after it has erased the person's rows of the other
// owners. Their checks are the use cases' (internal/usecase/customfieldretyping and
// internal/usecase/contacterasure), the one kind of package easement lets import this one. The
// contacts module does not import it.
package contactsunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// CustomFieldUpdate is the edit of one Custom field, its name and its type, opened on its row under
// the Workspace's lock (ADR 0108) so that the use case runs the Removal check of a retype between
// opening and applying it, on the field the edit read.
type CustomFieldUpdate struct {
	l     *ent.WorkspaceLocked
	field *ent.CustomField
	in    contacts.CustomFieldUpdateInput
	name  patch.Field[string]
}

// CustomFieldUpdateIn opens the edit in of the Custom field with the id in the transaction l holds
// the Workspace's lock in, and refuses what the field's own rules refuse before anything is
// written: an unknown id is contacts.ErrCustomFieldNotFound, a new name the name rule refuses is
// its refusal (validation.Name), and an edit made on a revision the field has moved past is
// contacts.ErrCustomFieldRevisionChanged (ADR 0119). Every write of a Custom field holds the lock,
// so the field read here is the one Apply replaces.
func CustomFieldUpdateIn(ctx context.Context, l *ent.WorkspaceLocked, id int64, in contacts.CustomFieldUpdateInput) (*CustomFieldUpdate, error) {
	field, err := contacts.CustomField(ctx, l.Scope(), id)
	if err != nil {
		return nil, err
	}
	name, err := validation.NamePatch(in.Name)
	if err != nil {
		return nil, err
	}
	if field.Revision != in.Revision {
		return nil, contacts.ErrCustomFieldRevisionChanged
	}
	return &CustomFieldUpdate{l: l, field: field, in: in, name: name}, nil
}

// Key is the field's key, by which Contacts' values and Condition leaves address it.
func (u *CustomFieldUpdate) Key() string { return u.field.Key }

// Retype returns the type the edit changes the field to, and false when the edit keeps its type.
func (u *CustomFieldUpdate) Retype() (customfield.Type, bool) {
	raw, ok := u.in.Type.Get()
	if !ok || customfield.Type(raw) == u.field.Type {
		return "", false
	}
	return customfield.Type(raw), true
}

// Apply renames the field or changes its type, an audited change (ADR 0022), and returns the field
// as edited. A type change rewrites no stored value: the leaves of the new type read the values as
// they are, and are false on one of another shape. It does not ask whether a Segment, a Rule or a
// Broadcast tests the field with a leaf the new type does not take: that is the use case's Removal
// check, run under the same lock before.
func (u *CustomFieldUpdate) Apply(ctx context.Context) (*ent.CustomField, error) {
	upd := u.l.CustomField().UpdateOneID(u.field.ID)
	patch.ApplySet(u.name, upd.SetName)
	if t, ok := u.Retype(); ok {
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
