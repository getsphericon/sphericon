package contacts

import (
	"context"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactlock"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// A Contact's Tags are its own (GLOSSARY: a Tag is an attribute of a Contact): applying and
// removing one writes the Contact's row and its join rows, so the change lives here, in the module
// that owns them (ADR 0102); the catalogue of Tags is internal/domain/contacts/tags. The bus opens
// the transaction a change of a Contact's Tags commits in with the System event that records it
// (contact.tagged, contact.untagged). A caller that already holds a transaction uses ApplyTagIn,
// RemoveTagIn and SetTagsIn instead. The refusals are the catalogue's (tags.ErrContactNotFound, and
// the name rule of tags.EnsureIn), as when the change lived there.

// ApplyTag gives the Contact the named Tag in a transaction of its own (ApplyTagIn).
func (m *Module) ApplyTag(ctx context.Context, s *ent.Scoped, contactID int64, name string) (*ent.Tag, error) {
	var t *ent.Tag
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		var err error
		t, err = ApplyTagIn(ctx, ts, pub, contactID, name)
		return err
	})
	return t, err
}

// RemoveTag takes the named Tag off the Contact in a transaction of its own (RemoveTagIn).
func (m *Module) RemoveTag(ctx context.Context, s *ent.Scoped, contactID int64, name string) error {
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		return RemoveTagIn(ctx, ts, pub, contactID, name)
	})
}

// ApplyTagIn gives the Contact the named Tag, creating the Tag on first use (tags.EnsureIn), and
// publishes contact.tagged, inside the transaction s and pub are bound to. It is idempotent:
// applying a Tag the Contact already has changes and records nothing. The Contact is checked before
// anything is written, so a failed apply creates no Tag; the name is the catalogue's to check.
func ApplyTagIn(ctx context.Context, s *ent.Scoped, pub events.Publisher, contactID int64, name string) (*ent.Tag, error) {
	c, err := lockTagged(ctx, s, contactID)
	if err != nil {
		return nil, err
	}
	t, err := tags.EnsureIn(ctx, s, name)
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
	return t, publishTagChange(ctx, s, pub, events.NameContactTagged, c, t)
}

// RemoveTagIn takes the named Tag off the Contact and publishes contact.untagged, inside the
// transaction s and pub are bound to. It is idempotent: a Tag the Contact does not have (or that
// does not exist) changes and records nothing. The Tag stays in the catalogue.
func RemoveTagIn(ctx context.Context, s *ent.Scoped, pub events.Publisher, contactID int64, name string) error {
	c, err := lockTagged(ctx, s, contactID)
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
	// Same touch of updated_at as ApplyTagIn, in the same transaction as the join row.
	if err := events.Unaudited(s).Contact().UpdateOneID(contactID).RemoveTagIDs(t.ID).Exec(ctx); err != nil {
		return err
	}
	return publishTagChange(ctx, s, pub, events.NameContactUntagged, c, t)
}

// SetTagsIn makes the named Tags the Contact's complete set inside the transaction s and pub are
// bound to: it applies the missing ones (creating them on first use) and removes the rest, each
// change recorded as ApplyTagIn and RemoveTagIn do. Blank and repeated names are dropped, so a
// form's empty chip changes nothing.
func SetTagsIn(ctx context.Context, s *ent.Scoped, pub events.Publisher, contactID int64, names []string) error {
	want := lo.Uniq(lo.FilterMap(names, func(n string, _ int) (string, bool) {
		n = strings.TrimSpace(n)
		return n, n != ""
	}))
	if _, err := lockTagged(ctx, s, contactID); err != nil {
		return err
	}
	have, err := s.Tag().Query().Where(tag.HasContactsWith(contact.ID(contactID))).All(ctx)
	if err != nil {
		return err
	}
	haveNames := lo.Map(have, func(t *ent.Tag, _ int) string { return t.Name })
	for _, name := range lo.Without(want, haveNames...) {
		if _, err := ApplyTagIn(ctx, s, pub, contactID, name); err != nil {
			return err
		}
	}
	for _, name := range lo.Without(haveNames, want...) {
		if err := RemoveTagIn(ctx, s, pub, contactID, name); err != nil {
			return err
		}
	}
	return nil
}

// lockTagged locks the Contact's row to the end of the transaction s is bound to (contactlock), so
// two changes of its Tags are ordered and each reads the set the other committed: an apply racing
// another apply of the same Tag records it once. tags.ErrContactNotFound when the Contact is not
// in the Workspace.
func lockTagged(ctx context.Context, s *ent.Scoped, contactID int64) (*ent.Contact, error) {
	c, err := contactlock.Lock(ctx, s, contactID)
	if ent.IsNotFound(err) {
		return nil, tags.ErrContactNotFound
	}
	return c, err
}

// hasTag reports whether the Contact has the Tag.
func hasTag(ctx context.Context, s *ent.Scoped, contactID, tagID int64) (bool, error) {
	return s.Tag().Query().Where(tag.ID(tagID), tag.HasContactsWith(contact.ID(contactID))).Exist(ctx)
}

// publishTagChange records the change of the Contact's Tags as the System event action.
func publishTagChange(ctx context.Context, s *ent.Scoped, pub events.Publisher, action string, c *ent.Contact, t *ent.Tag) error {
	return pub.Publish(ctx, &events.ContactTagChange{
		Action: action, WorkspaceID: s.WorkspaceID(), ContactID: c.ID, Email: lo.FromPtr(c.Email), Tag: t.Name,
	})
}
