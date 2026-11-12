package contacts

import (
	"context"

	"github.com/getsphericon/sphericon/internal/kernel/tagname"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/contacttag"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactlock"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// A Contact's Tags are its own (GLOSSARY: a Tag is an attribute of a Contact): applying and
// removing one writes the Contact's row and its join rows, so the change lives here, in the module
// that owns them (ADR 0102); the catalogue of Tags is internal/domain/contacts/tags. The bus opens
// the transaction a change of a Contact's Tags commits in with the System event that records it
// (contact.tagged, contact.untagged). A caller that already holds a transaction uses ApplyTagIn,
// RemoveTagIn and SetTagsIn instead. A Contact not in the Workspace is ErrNotFound; the Tag name is
// the catalogue's to check (tags.EnsureIn).

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
	if err := s.ContactTag().Create().SetContactID(contactID).SetTagID(t.ID).Exec(ctx); err != nil {
		return nil, err
	}
	if err := touch(ctx, s, contactID); err != nil {
		return nil, err
	}
	return t, publishTagChange(ctx, s, pub, systemevent.ContactTagged, c, t)
}

// RemoveTagIn takes the named Tag off the Contact and publishes contact.untagged, inside the
// transaction s and pub are bound to. It is idempotent: a Tag the Contact does not have (or that
// does not exist) changes and records nothing. The Tag stays in the catalogue.
func RemoveTagIn(ctx context.Context, s *ent.Scoped, pub events.Publisher, contactID int64, name string) error {
	c, err := lockTagged(ctx, s, contactID)
	if err != nil {
		return err
	}
	t, err := s.Tag().Query().Where(tags.Named(name)).Only(ctx)
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
	if _, err := s.ContactTag().Delete().Where(contacttag.ContactID(contactID), contacttag.TagID(t.ID)).Exec(ctx); err != nil {
		return err
	}
	if err := touch(ctx, s, contactID); err != nil {
		return err
	}
	return publishTagChange(ctx, s, pub, systemevent.ContactUntagged, c, t)
}

// SetTagsIn makes the named Tags the Contact's complete set inside the transaction s and pub are
// bound to: it applies the missing ones (creating them on first use) and removes the rest, each
// change recorded as ApplyTagIn and RemoveTagIn do. Names are compared as Tags are matched
// (tagname): a name the Contact holds in another case is kept, not removed and applied again, and
// blank and repeated names are dropped, so a form's empty chip changes nothing.
func SetTagsIn(ctx context.Context, s *ent.Scoped, pub events.Publisher, contactID int64, names []string) error {
	want := lo.UniqBy(lo.Filter(names, func(n string, _ int) bool { return tagname.Key(n) != "" }), tagname.Key)
	if _, err := lockTagged(ctx, s, contactID); err != nil {
		return err
	}
	have, err := s.Tag().Query().Where(tag.HasContactsWith(contact.ID(contactID))).All(ctx)
	if err != nil {
		return err
	}
	wanted := lo.SliceToMap(want, func(n string) (string, bool) { return tagname.Key(n), true })
	held := lo.SliceToMap(have, func(t *ent.Tag) (string, bool) { return t.NameKey, true })
	for _, name := range want {
		if held[tagname.Key(name)] {
			continue
		}
		if _, err := ApplyTagIn(ctx, s, pub, contactID, name); err != nil {
			return err
		}
	}
	for _, t := range have {
		if wanted[t.NameKey] {
			continue
		}
		if err := RemoveTagIn(ctx, s, pub, contactID, t.Name); err != nil {
			return err
		}
	}
	return nil
}

// lockTagged locks the Contact's row to the end of the transaction s is bound to (contactlock), so
// two changes of its Tags are ordered and each reads the set the other committed: an apply racing
// another apply of the same Tag records it once. ErrNotFound when the Contact is not
// in the Workspace.
func lockTagged(ctx context.Context, s *ent.Scoped, contactID int64) (*ent.Contact, error) {
	locked, err := contactlock.Lock(ctx, s, contactID)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return locked.Contact(), err
}

// hasTag reports whether the Contact has the Tag.
func hasTag(ctx context.Context, s *ent.Scoped, contactID, tagID int64) (bool, error) {
	return s.ContactTag().Query().Where(contacttag.ContactID(contactID), contacttag.TagID(tagID)).Exist(ctx)
}

// touch moves the Contact's updated_at (TimeMixin's UpdateDefault) in the transaction of the
// change of its join rows, so the Contacts Export stream re-delivers it.
func touch(ctx context.Context, s *ent.Scoped, contactID int64) error {
	return events.Unaudited(s).Contact().UpdateOneID(contactID).Exec(ctx)
}

// publishTagChange records the change of the Contact's Tags as the System event action.
func publishTagChange(ctx context.Context, s *ent.Scoped, pub events.Publisher, action string, c *ent.Contact, t *ent.Tag) error {
	return pub.Publish(ctx, &events.ContactTagChange{
		Action: action, WorkspaceID: s.WorkspaceID(), ContactID: c.ID, Email: lo.FromPtr(c.Email), Tag: t.Name,
	})
}
