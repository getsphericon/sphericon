// Package tags is the one place a Contact's Tags are listed, applied and removed
// (ADR 0016). A Tag is a presence-only, workspace-scoped label (GLOSSARY); it is
// auto-created the first time it is applied, like a Custom field. Handlers on /site
// and /api stay thin adapters: scope check, call, map. Tag-based targeting is not
// here — it is the has-Tag rule of the segments engine.
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
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/validation"
)

// Domain errors. Callers match with errors.Is.
var (
	// ErrContactNotFound: no such Contact in this Workspace.
	ErrContactNotFound = validation.Refuse("tags: contact not found", "errors.contact_not_found")
	// ErrInvalidName: the Tag name is blank.
	ErrInvalidName = validation.Refuse("tags: name is required", "errors.name_required")
	// ErrNotFound: no such Tag in this Workspace.
	ErrNotFound = validation.Refuse("tags: tag not found", "errors.tag_not_found")
)

// Module is the tags module. It holds no state: every call receives the
// Workspace-scoped client (ADR 0017) built by the entry point, so the module
// writes no Workspace predicate and cannot name another Workspace.
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

// Apply gives the Contact the named Tag, creating the Tag on first use. It is
// idempotent: applying a Tag the Contact already has changes nothing. The Contact
// is checked before anything is written, so a failed apply creates no Tag.
func (m *Module) Apply(ctx context.Context, s *ent.Scoped, contactID int64, name string) (*ent.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrInvalidName
	}
	if err := requireContact(ctx, s, contactID); err != nil {
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
	// An update by id moves the Contact's updated_at (TimeMixin's UpdateDefault) in the
	// same transaction as the join row, so the Contacts Export stream re-delivers it.
	if err := events.Unaudited(s).Contact().UpdateOneID(contactID).AddTagIDs(t.ID).Exec(ctx); err != nil {
		return nil, err
	}
	return t, nil
}

// Set makes the named Tags the Contact's complete set: it applies the missing ones
// (creating them on first use) and removes the rest. Blank and repeated names are
// dropped, so a form's empty chip changes nothing.
func (m *Module) Set(ctx context.Context, s *ent.Scoped, contactID int64, names []string) error {
	want := lo.Uniq(lo.FilterMap(names, func(n string, _ int) (string, bool) {
		n = strings.TrimSpace(n)
		return n, n != ""
	}))
	have, err := m.ForContact(ctx, s, contactID)
	if err != nil {
		return err
	}
	haveNames := lo.Map(have, func(t *ent.Tag, _ int) string { return t.Name })
	for _, name := range lo.Without(want, haveNames...) {
		if _, err := m.Apply(ctx, s, contactID, name); err != nil {
			return err
		}
	}
	for _, name := range lo.Without(haveNames, want...) {
		if err := m.Remove(ctx, s, contactID, name); err != nil {
			return err
		}
	}
	return nil
}

// Remove takes the named Tag off the Contact. It is idempotent: a Tag the Contact
// does not have (or that does not exist) is a no-op. The Tag stays in the catalogue.
func (m *Module) Remove(ctx context.Context, s *ent.Scoped, contactID int64, name string) error {
	if err := requireContact(ctx, s, contactID); err != nil {
		return err
	}
	t, err := s.Tag().Query().Where(tag.Name(strings.TrimSpace(name))).Only(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	// Same touch of updated_at as Apply, in the same transaction as the join row.
	return events.Unaudited(s).Contact().UpdateOneID(contactID).RemoveTagIDs(t.ID).Exec(ctx)
}

// Delete removes a Tag from the catalogue, and from every Contact that has it, unless a Segment
// rule or an Automation step names it, an archived Automation's too: then it returns an *inuse.Error naming those Segments and
// Automations (ADR 0049), because both name the Tag and would silently stop matching or applying.
// A missing Tag is ErrNotFound.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	t, err := s.Tag().Get(ctx, id)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var users []inuse.User
	segs, err := segments.UsingTag(ctx, s, t.Name)
	if err != nil {
		return err
	}
	for _, seg := range segs {
		users = append(users, inuse.User{Kind: inuse.Segment, ID: seg.ID, Name: seg.Name})
	}
	autos, err := automations.UsingTag(ctx, s, t.Name)
	if err != nil {
		return err
	}
	for _, a := range autos {
		users = append(users, inuse.User{Kind: inuse.Automation, ID: a.ID, Name: a.Name})
	}
	live, err := rules.UsingTag(ctx, s, t.Name)
	if err != nil {
		return err
	}
	users = append(users, inuse.Rules(live)...)
	if len(users) > 0 {
		return &inuse.Error{What: inuse.SubjectTag, Users: users}
	}
	return s.Tag().DeleteOneID(id).Exec(ctx)
}

func requireContact(ctx context.Context, s *ent.Scoped, contactID int64) error {
	_, err := s.Contact().Get(ctx, contactID)
	if ent.IsNotFound(err) {
		return ErrContactNotFound
	}
	return err
}
