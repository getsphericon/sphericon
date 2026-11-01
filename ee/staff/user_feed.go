package staff

import (
	"cmp"
	"context"
	"slices"

	"github.com/getsphericon/sphericon/ent"
	entuser "github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/ent/usertombstone"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// UserFeedEntry is a User as the feed reports it: the User with their current consent to the
// operator's mailings, or the tombstone of a deleted User.
type UserFeedEntry struct {
	// User is the User; nil on a tombstone.
	User *ent.User
	// Tombstone is what is left of a deleted User; nil while the User exists.
	Tombstone *ent.UserTombstone
	// Mailing is the User's latest decision on the item that stands for consent to the
	// operator's mailings; nil when there is none, and on a tombstone.
	Mailing *consentrecords.Mailing
}

// UserID is the id of the User the entry reports.
func (e UserFeedEntry) UserID() int64 {
	if e.Tombstone != nil {
		return e.Tombstone.UserID
	}
	return e.User.ID
}

func (e UserFeedEntry) position() int64 {
	if e.Tombstone != nil {
		return e.Tombstone.FeedPosition
	}
	return e.User.FeedPosition
}

// UserFeed is the Users created or changed after cursor, and the tombstones of those deleted
// after it, in the order of their change position (then User id), at most limit of them, each
// User with their current consent to the operator's mailings (ADR 0062). An empty cursor starts
// from the beginning. Like the Workspace feed, the module keeps no position: reading is
// idempotent, the caller advances by saving NextCursor, and delivery is at-least-once; a User
// changed again after it was read is listed again, as they are by then. It reads the Users and
// their tombstones through the raw client, and the consents through internal/consentrecords.
func (m *Module) UserFeed(ctx context.Context, cursor string, limit int) (pagination.CursorPage[UserFeedEntry], error) {
	if err := m.licensed(); err != nil {
		return pagination.CursorPage[UserFeedEntry]{}, err
	}
	position, id, err := parseFeedCursor(cursor)
	if err != nil {
		return pagination.CursorPage[UserFeedEntry]{}, err
	}
	users, err := m.ent.User.Query().
		Where(entuser.Or(
			entuser.FeedPositionGT(position),
			entuser.And(entuser.FeedPositionEQ(position), entuser.IDGT(id)),
		)).
		Order(ent.Asc(entuser.FieldFeedPosition), ent.Asc(entuser.FieldID)).
		Limit(limit + 1).
		All(ctx)
	if err != nil {
		return pagination.CursorPage[UserFeedEntry]{}, err
	}
	tombstones, err := m.ent.UserTombstone.Query().
		Where(usertombstone.Or(
			usertombstone.FeedPositionGT(position),
			usertombstone.And(usertombstone.FeedPositionEQ(position), usertombstone.UserIDGT(id)),
		)).
		Order(ent.Asc(usertombstone.FieldFeedPosition), ent.Asc(usertombstone.FieldUserID)).
		Limit(limit + 1).
		All(ctx)
	if err != nil {
		return pagination.CursorPage[UserFeedEntry]{}, err
	}

	// A User id is never reused, so (position, User id) orders the two sources as one.
	entries := make([]UserFeedEntry, 0, len(users)+len(tombstones))
	for _, u := range users {
		entries = append(entries, UserFeedEntry{User: u})
	}
	for _, t := range tombstones {
		entries = append(entries, UserFeedEntry{Tombstone: t})
	}
	slices.SortFunc(entries, func(a, b UserFeedEntry) int {
		return cmp.Or(cmp.Compare(a.position(), b.position()), cmp.Compare(a.UserID(), b.UserID()))
	})
	page := pagination.Cursor(entries, limit, cursor, func(e UserFeedEntry) string { return feedCursor(e.position(), e.UserID()) })

	var live []int64
	for _, e := range page.Items {
		if e.User != nil {
			live = append(live, e.User.ID)
		}
	}
	mailings, err := consentrecords.Mailings(ctx, m.ent, live...)
	if err != nil {
		return pagination.CursorPage[UserFeedEntry]{}, err
	}
	for i := range page.Items {
		if mailing, ok := mailings[page.Items[i].UserID()]; ok && page.Items[i].User != nil {
			page.Items[i].Mailing = &mailing
		}
	}
	return page, nil
}
