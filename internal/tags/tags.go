// Package tags is the one place a Contact's Tags are listed, applied and removed
// (ADR 0016). A Tag is a presence-only, workspace-scoped label (GLOSSARY); it is
// auto-created the first time it is applied, like a Custom field. Handlers on /site
// and /api stay thin adapters: scope check, call, map. Tag-based targeting is not
// here — it is the has-Tag rule of the segments engine. Deleting a Tag from the
// catalogue is the use case internal/usecase/tagdeletion (ADR 0102): the modules
// that name a Tag sit above this one, so the check of their Live use is not here.
package tags

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/contactlock"
	"github.com/getsphericon/sphericon/internal/infra/events"
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
// Workspace; the bus opens the transaction a change of a Contact's Tags commits in with the
// System event that records it (contact.tagged, contact.untagged). A caller that already holds a
// transaction uses ApplyIn, RemoveIn and SetIn instead.
type Module struct {
	bus *events.Bus
}

// New builds the tags module.
func New(bus *events.Bus) *Module { return &Module{bus: bus} }

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

// Apply gives the Contact the named Tag in a transaction of its own (ApplyIn).
func (m *Module) Apply(ctx context.Context, s *ent.Scoped, contactID int64, name string) (*ent.Tag, error) {
	var t *ent.Tag
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		var err error
		t, err = ApplyIn(ctx, ts, pub, contactID, name)
		return err
	})
	return t, err
}

// Remove takes the named Tag off the Contact in a transaction of its own (RemoveIn).
func (m *Module) Remove(ctx context.Context, s *ent.Scoped, contactID int64, name string) error {
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		return RemoveIn(ctx, ts, pub, contactID, name)
	})
}

// ApplyIn gives the Contact the named Tag, creating the Tag on first use, and publishes
// contact.tagged, inside the transaction s and pub are bound to. It is idempotent: applying a Tag
// the Contact already has changes and records nothing. The Contact is checked before anything is
// written, so a failed apply creates no Tag.
func ApplyIn(ctx context.Context, s *ent.Scoped, pub events.Publisher, contactID int64, name string) (*ent.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrInvalidName
	}
	c, err := lockContact(ctx, s, contactID)
	if err != nil {
		return nil, err
	}
	// DO NOTHING, not Ignore: an insert is the audited create of the Tag (ADR 0022),
	// a conflict writes nothing and reports sql.ErrNoRows.
	if err := s.Tag().Create().
		SetName(name).
		OnConflictColumns(tag.FieldName, tag.FieldWorkspaceID).
		DoNothing().
		Exec(ctx); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	t, err := s.Tag().Query().Where(tag.Name(name)).Only(ctx)
	if err != nil {
		return nil, err
	}
	has, err := hasTag(ctx, s, contactID, t.ID)
	if err != nil || has {
		return t, err
	}
	// An update by id moves the Contact's updated_at (TimeMixin's UpdateDefault) in the
	// same transaction as the join row, so the Contacts Export stream re-delivers it.
	if err := events.Unaudited(s).Contact().UpdateOneID(contactID).AddTagIDs(t.ID).Exec(ctx); err != nil {
		return nil, err
	}
	return t, publishChange(ctx, s, pub, events.NameContactTagged, c, t)
}

// RemoveIn takes the named Tag off the Contact and publishes contact.untagged, inside the
// transaction s and pub are bound to. It is idempotent: a Tag the Contact does not have (or that
// does not exist) changes and records nothing. The Tag stays in the catalogue.
func RemoveIn(ctx context.Context, s *ent.Scoped, pub events.Publisher, contactID int64, name string) error {
	c, err := lockContact(ctx, s, contactID)
	if err != nil {
		return err
	}
	t, err := s.Tag().Query().Where(tag.Name(strings.TrimSpace(name))).Only(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	has, err := hasTag(ctx, s, contactID, t.ID)
	if err != nil || !has {
		return err
	}
	// Same touch of updated_at as ApplyIn, in the same transaction as the join row.
	if err := events.Unaudited(s).Contact().UpdateOneID(contactID).RemoveTagIDs(t.ID).Exec(ctx); err != nil {
		return err
	}
	return publishChange(ctx, s, pub, events.NameContactUntagged, c, t)
}

// SetIn makes the named Tags the Contact's complete set inside the transaction s and pub are
// bound to: it applies the missing ones (creating them on first use) and removes the rest, each
// change recorded as ApplyIn and RemoveIn do. Blank and repeated names are dropped, so a form's
// empty chip changes nothing.
func SetIn(ctx context.Context, s *ent.Scoped, pub events.Publisher, contactID int64, names []string) error {
	want := lo.Uniq(lo.FilterMap(names, func(n string, _ int) (string, bool) {
		n = strings.TrimSpace(n)
		return n, n != ""
	}))
	if _, err := lockContact(ctx, s, contactID); err != nil {
		return err
	}
	have, err := s.Tag().Query().Where(tag.HasContactsWith(contact.ID(contactID))).All(ctx)
	if err != nil {
		return err
	}
	haveNames := lo.Map(have, func(t *ent.Tag, _ int) string { return t.Name })
	for _, name := range lo.Without(want, haveNames...) {
		if _, err := ApplyIn(ctx, s, pub, contactID, name); err != nil {
			return err
		}
	}
	for _, name := range lo.Without(haveNames, want...) {
		if err := RemoveIn(ctx, s, pub, contactID, name); err != nil {
			return err
		}
	}
	return nil
}

// lockContact locks the Contact's row to the end of the transaction s is bound to (contactlock), so
// two changes of its Tags are ordered and each reads the set the other committed: an apply racing
// another apply of the same Tag records it once. ErrContactNotFound when the Contact is not in the
// Workspace.
func lockContact(ctx context.Context, s *ent.Scoped, contactID int64) (*ent.Contact, error) {
	c, err := contactlock.Lock(ctx, s, contactID)
	if ent.IsNotFound(err) {
		return nil, ErrContactNotFound
	}
	return c, err
}

// hasTag reports whether the Contact has the Tag.
func hasTag(ctx context.Context, s *ent.Scoped, contactID, tagID int64) (bool, error) {
	return s.Tag().Query().Where(tag.ID(tagID), tag.HasContactsWith(contact.ID(contactID))).Exist(ctx)
}

// publishChange records the change of the Contact's Tags as the System event action.
func publishChange(ctx context.Context, s *ent.Scoped, pub events.Publisher, action string, c *ent.Contact, t *ent.Tag) error {
	return pub.Publish(ctx, &events.ContactTagChange{
		Action: action, WorkspaceID: s.WorkspaceID(), ContactID: c.ID, Email: lo.FromPtr(c.Email), Tag: t.Name,
	})
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
// internal/usecase/tagdeletion, which runs that Live use check and this removal in one transaction
// under the Workspace's lock; easement forbids every other package to call DeleteIn. A missing Tag is
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
