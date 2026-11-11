// Package customfieldretyping is the use case that edits a Custom field, its name and its type
// (ADR 0102, ADR 0006). A Custom field is tested by the modules above the contacts module, in
// Definition use: retyping it runs the Removal check (internal/domain/orchestration/removalcheck),
// which refuses with a *liveuse.Error naming what tests the field with a leaf the new type does not
// take, because their Conditions would stop compiling. The contacts module knows none of them, so
// the check runs here before the module's unchecked edit, contactsunchecked.UpdateCustomFieldIn,
// which only a use case may import (ADR 0110). The check and the edit run in one transaction
// under the Workspace's lock (ADR 0099).
package customfieldretyping

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts/contactsunchecked"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/removalcheck"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// UseCase edits a Custom field. Every call receives the Workspace-scoped client (ADR 0017) built by
// the entry point; the use case opens the transaction the modules it calls join.
type UseCase struct {
	bus   *events.Bus
	check *removalcheck.Module
}

// New builds the Custom field retyping use case over the Removal check.
func New(bus *events.Bus, check *removalcheck.Module) *UseCase {
	return &UseCase{bus: bus, check: check}
}

// Update renames a Custom field or changes its type, an audited change (ADR 0022). A type change
// rewrites no stored value: the leaves of the new type read the values as they are, and are false
// on one of another shape. It is refused with the Removal check's *liveuse.Error while a Segment, a
// kept Rule or an armed Broadcast tests the field with a leaf the new type does not take, as their
// Conditions would stop compiling; a leaf every type takes (notNull) does not hold the type. An
// unknown id is contacts.ErrCustomFieldNotFound, a blank new name contacts.ErrCustomFieldNameRequired.
//
// The check and the edit run in one transaction under the Workspace's lock (ADR 0099), which saving
// a Segment or a Rule and scheduling a Broadcast take too: one saved first is what the check finds,
// and one saved after is checked against the new type. A rename takes the same path.
func (u *UseCase) Update(ctx context.Context, s *ent.Scoped, id int64, in contacts.CustomFieldUpdateInput) (*ent.CustomField, error) {
	var updated *ent.CustomField
	err := u.bus.WithinWorkspaceLockTx(ctx, s, func(wl *ent.WorkspaceLocked, _ events.Publisher) error {
		field, err := contacts.CustomField(ctx, wl.Scope(), id)
		if err != nil {
			return err
		}
		if err := in.Check(); err != nil {
			return err
		}
		if t, ok := in.Retype(field); ok {
			if err := u.check.CustomFieldRetype(ctx, wl, field.Key, t); err != nil {
				return err
			}
		}
		updated, err = contactsunchecked.UpdateCustomFieldIn(ctx, wl, field, in)
		return err
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}
