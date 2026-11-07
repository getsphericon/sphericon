// Package customfieldretyping is the use case that edits a Custom field, its name and its type
// (ADR 0102, ADR 0006). A Custom field is tested by the modules above the contacts module: a
// Segment's Condition, a kept Rule and the Condition an armed Broadcast copied from its Segment.
// Retyping it asks each of them whether it tests the field with a leaf the new type does not take,
// and refuses with a *liveuse.Error naming them if one does, because their Conditions would stop
// compiling. The contacts module knows none of them, so the check lives here and calls the
// module's unchecked edit, contacts.UpdateCustomFieldIn, which easement forbids every other package
// to call. The check and the edit run in one transaction under the Workspace's lock (ADR 0099).
package customfieldretyping

import (
	"context"
	"slices"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// UseCase edits a Custom field. Every call receives the Workspace-scoped client (ADR 0017) built by
// the entry point; the use case opens the transaction the modules it calls join.
type UseCase struct {
	bus *events.Bus
}

// New builds the Custom field retyping use case.
func New(bus *events.Bus) *UseCase { return &UseCase{bus: bus} }

// Update renames a Custom field or changes its type, an audited change (ADR 0022). A type change
// rewrites no stored value: the leaves of the new type read the values as they are, and are false
// on one of another shape. It is refused with a *liveuse.Error while a Segment, a kept Rule or an
// armed Broadcast tests the field with a leaf the new type does not take, as their Conditions would
// stop compiling; a leaf every type takes (notNull) does not hold the type. An unknown id is
// contacts.ErrCustomFieldNotFound, a blank new name contacts.ErrCustomFieldNameRequired.
//
// The check and the edit run in one transaction under the Workspace's lock (ADR 0099), which saving
// a Segment or a Rule and scheduling a Broadcast take too: one saved first is what the check finds,
// and one saved after is checked against the new type. A rename takes the same path.
func (u *UseCase) Update(ctx context.Context, s *ent.Scoped, id int64, in contacts.CustomFieldUpdateInput) (*ent.CustomField, error) {
	var updated *ent.CustomField
	err := u.bus.WithinWorkspaceLockTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		field, err := contacts.CustomField(ctx, ts, id)
		if err != nil {
			return err
		}
		if err := in.Check(); err != nil {
			return err
		}
		if t, ok := in.Retype(field); ok {
			users, err := unfitUsers(ctx, ts, field.Key, t)
			if err != nil {
				return err
			}
			if len(users) > 0 {
				return &liveuse.Error{What: liveuse.SubjectCustomField, Users: users}
			}
		}
		updated, err = contacts.UpdateCustomFieldIn(ctx, ts, field, in)
		return err
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// unfitUsers is what tests the Custom field key with a Condition leaf a field of type t does not
// take: Segments, kept Rules and armed Broadcasts. An Automation holds no Condition.
func unfitUsers(ctx context.Context, s *ent.Scoped, key string, t customfield.Type) ([]liveuse.User, error) {
	segs, err := segments.UnfitForCustomFieldType(ctx, s, key, t)
	if err != nil {
		return nil, err
	}
	kept, err := rules.UnfitForCustomFieldType(ctx, s, key, t)
	if err != nil {
		return nil, err
	}
	armed, err := broadcasts.UnfitForCustomFieldType(ctx, s, key, t)
	if err != nil {
		return nil, err
	}
	return slices.Concat(liveuse.Segments(segs), liveuse.Rules(kept), liveuse.Broadcasts(armed)), nil
}
