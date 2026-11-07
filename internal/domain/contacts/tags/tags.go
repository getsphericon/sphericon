// Package tags is the catalogue of Tags and the one place a Contact's Tags are listed
// (ADR 0016). A Tag is a presence-only, workspace-scoped label (GLOSSARY); it is
// auto-created the first time it is applied (EnsureIn), like a Custom field. Applying
// and removing one writes the Contact, so it is the contacts module's (contacts.ApplyTagIn,
// RemoveTagIn, SetTagsIn, ADR 0102: only the owner writes a table), which returns this
// package's refusals. Handlers on /site and /api stay thin adapters: scope check, call,
// map. Tag-based targeting is not here — it is the has-Tag rule of the segments engine. Deleting a Tag from the
// catalogue is the use case internal/usecase/tagdeletion (ADR 0102): the modules
// that name a Tag sit above this one, so the check of their Live use is not here.
package tags

import (
	"context"
	"database/sql"
	"errors"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Domain errors. Callers match with errors.Is.
var (
	// ErrContactNotFound: no such Contact in this Workspace.
	ErrContactNotFound = validation.NotFound("tags: contact not found", i18n.ErrorsContactNotFound)
	// ErrInvalidName: the Tag name is blank.
	ErrInvalidName = validation.Refuse("tags: name is required", i18n.ErrorsNameRequired)
	// ErrNotFound: no such Tag in this Workspace.
	ErrNotFound = validation.NotFound("tags: tag not found", i18n.ErrorsTagNotFound)
)

// Module is the tags module. Every call receives the Workspace-scoped client (ADR 0017) built
// by the entry point, so the module writes no Workspace predicate and cannot name another
// Workspace.
type Module struct{}

// New builds the tags module.
func New() *Module { return &Module{} }

// List returns the Workspace's Tag catalogue ordered by name.
func (m *Module) List(ctx context.Context, s *ent.Scoped) ([]*ent.Tag, error) {
	return s.Tag().Query().Order(ent.Asc(tag.FieldName)).All(ctx)
}

// ForContact returns the Tags a Contact has, ordered by name.
func (m *Module) ForContact(ctx context.Context, s *ent.Scoped, contactID int64) ([]*ent.Tag, error) {
	if err := requireContact(ctx, s, contactID); err != nil {
		return nil, err
	}
	return s.Tag().Query().
		Where(tag.HasContactsWith(contact.ID(contactID))).
		Order(ent.Asc(tag.FieldName)).
		All(ctx)
}

// EnsureIn returns the Tag named name, creating it on first use, inside the transaction s is
// bound to: applying a Tag to a Contact (contacts.ApplyTagIn) auto-creates it, like a Custom field.
func EnsureIn(ctx context.Context, s *ent.Scoped, name string) (*ent.Tag, error) {
	// DO NOTHING, not Ignore: an insert is the audited create of the Tag (ADR 0022),
	// a conflict writes nothing and reports sql.ErrNoRows.
	if err := s.Tag().Create().
		SetName(name).
		OnConflictColumns(tag.FieldName, tag.FieldWorkspaceID).
		DoNothing().
		Exec(ctx); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return s.Tag().Query().Where(tag.Name(name)).Only(ctx)
}

// Get returns the Tag with the given id, ErrNotFound when the Workspace has none.
func Get(ctx context.Context, s *ent.Scoped, id int64) (*ent.Tag, error) {
	t, err := s.Tag().Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return t, err
}

// DeleteIn removes the Tag from the catalogue, and from every Contact that has it, inside the
// transaction s is bound to. It is the unchecked removal (ADR 0102): it does not ask whether a
// Segment, an Automation, a Rule or a Broadcast names the Tag. Deleting a Tag is the use case
// internal/usecase/tagdeletion, which runs the Removal check (the Tag's Definition use) and this
// removal in one transaction under the Workspace's lock; easement forbids every other package to call DeleteIn. A missing Tag is
// ErrNotFound.
func DeleteIn(ctx context.Context, s *ent.Scoped, id int64) error {
	err := s.Tag().DeleteOneID(id).Exec(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

func requireContact(ctx context.Context, s *ent.Scoped, contactID int64) error {
	_, err := s.Contact().Get(ctx, contactID)
	if ent.IsNotFound(err) {
		return ErrContactNotFound
	}
	return err
}
