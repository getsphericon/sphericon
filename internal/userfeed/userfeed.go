// Package userfeed keeps the change position behind the service feed of Users (ADR 0062), which
// the operator's control plane reads to keep its mailing list in step. A User takes the next
// position whenever they are created, renamed, change email or verification, make a new
// decision on the Sign-up item that stands for consent to the operator's mailings, or that
// item's mark is set or cleared. A deleted User leaves a tombstone (UserTombstone) that takes
// the next position, because the User row is gone. The service endpoint reads Users and
// tombstones in the order of that position (ee/staff), so what a caller saved as its cursor
// always names a place in the order of changes.
//
// Two ent hooks keep it so no write path can forget it, the same pair as internal/workspacefeed.
// Hook runs on every transaction client (the event bus's): the change and its position commit
// together, in the order the positions were taken. Guard runs on the plain client
// (db.NewEntClient) and refuses a change to the feed outside a transaction, where the position
// would be a separate statement that a crash loses and a concurrent change commits out of
// order: every Consent record, whatever its item, is written in a transaction, so a writer that
// forgets it is refused before it looks at any row. The hook reads Consent records and Sign-up
// items to find the Users a change belongs to, which is why this package is on the closed list
// of raw-client packages in docs/architecture.md.
package userfeed

import (
	"context"
	"errors"
	"fmt"

	"entgo.io/ent"
	"github.com/samber/lo"

	entclient "github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/consentrecord"
	"github.com/getsphericon/sphericon/ent/legaldocumentversion"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/signupitem"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/ent/usertombstone"
	"github.com/getsphericon/sphericon/internal/postgres"
	"github.com/getsphericon/sphericon/internal/workspacefeed"
)

// ErrNeedsTransaction is what Guard returns for a change to the feed made on a client that is
// not bound to a transaction.
var ErrNeedsTransaction = errors.New("user feed change outside a transaction")

// Guard is the ent hook for a client that is not bound to a transaction: it refuses every
// mutation that Hook would give a position to. Such a change goes through the event bus's
// transaction (events.Bus.WithinTx).
func Guard(next ent.Mutator) ent.Mutator {
	return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
		if changesFeed(m) {
			return nil, ErrNeedsTransaction
		}
		return next.Mutate(ctx, m)
	})
}

// changesFeed reports whether m can change what the feed reports, before looking at any row:
// Hook narrows a Consent record to the mailings consent item.
func changesFeed(m ent.Mutation) bool {
	switch mu := m.(type) {
	case *entclient.UserMutation:
		return mu.Op().Is(ent.OpCreate|ent.OpDelete|ent.OpDeleteOne) || changesUser(mu)
	case *entclient.ConsentRecordMutation:
		return mu.Op().Is(ent.OpCreate)
	case *entclient.SignupItemMutation:
		return changesMark(mu)
	}
	return false
}

// changesUser reports whether m is an update of what the feed reports of a User: the name, the
// email and its verification.
func changesUser(m *entclient.UserMutation) bool {
	if !m.Op().Is(ent.OpUpdate | ent.OpUpdateOne) {
		return false
	}
	_, name := m.Name()
	_, email := m.Email()
	_, verified := m.EmailVerifiedAt()
	return name || email || verified || m.EmailVerifiedAtCleared()
}

// changesMark reports whether m is an update that sets or clears which item stands for consent
// to the operator's mailings: every User who answered it now has a different current consent.
func changesMark(m *entclient.SignupItemMutation) bool {
	if !m.Op().Is(ent.OpUpdate | ent.OpUpdateOne) {
		return false
	}
	_, marked := m.MailingsConsent()
	return marked
}

// Hook is the ent hook to install on a client bound to a transaction (Client.Use). It is a no-op
// for every mutation that cannot change what the feed reports.
func Hook(next ent.Mutator) ent.Mutator {
	return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
		switch mu := m.(type) {
		case *entclient.UserMutation:
			return mutateUser(ctx, next, mu)
		case *entclient.ConsentRecordMutation:
			return mutateConsent(ctx, next, mu)
		case *entclient.SignupItemMutation:
			return mutateMark(ctx, next, mu)
		}
		return next.Mutate(ctx, m)
	})
}

func mutateUser(ctx context.Context, next ent.Mutator, m *entclient.UserMutation) (ent.Value, error) {
	c := m.Client()
	switch {
	case m.Op().Is(ent.OpCreate):
		v, err := next.Mutate(ctx, m)
		if err != nil {
			return v, err
		}
		if id, ok := m.ID(); ok {
			err = touch(ctx, c, user.ID(id))
		}
		return v, err
	case m.Op().Is(ent.OpDelete | ent.OpDeleteOne):
		// The row is gone afterwards: its id is read first and kept on the tombstone.
		ids, err := m.IDs(ctx)
		if err != nil {
			return nil, err
		}
		v, err := next.Mutate(ctx, m)
		if err != nil || len(ids) == 0 {
			return v, err
		}
		return v, bury(ctx, c, ids)
	case changesUser(m):
		ids, err := m.IDs(ctx)
		if err != nil {
			return nil, err
		}
		v, err := next.Mutate(ctx, m)
		if err != nil || len(ids) == 0 {
			return v, err
		}
		return v, touch(ctx, c, user.IDIn(ids...))
	}
	return next.Mutate(ctx, m)
}

// mutateConsent puts the User back in the feed when the new record is a decision on the item
// that stands for consent to the operator's mailings.
func mutateConsent(ctx context.Context, next ent.Mutator, m *entclient.ConsentRecordMutation) (ent.Value, error) {
	if !m.Op().Is(ent.OpCreate) {
		return next.Mutate(ctx, m)
	}
	v, err := next.Mutate(ctx, m)
	if err != nil {
		return v, err
	}
	userID, _ := m.UserID()
	versionID, _ := m.VersionID()
	mailings, err := m.Client().LegalDocumentVersion.Query().
		Where(legaldocumentversion.ID(versionID), legaldocumentversion.HasItemWith(signupitem.MailingsConsent(true))).
		Exist(ctx)
	if err != nil || !mailings {
		return v, err
	}
	return v, touch(ctx, m.Client(), user.ID(userID))
}

// mutateMark puts back in the feed every User who answered an item whose mailings consent mark is
// set or cleared: what the feed reports as their consent is now another item's answer, or none.
func mutateMark(ctx context.Context, next ent.Mutator, m *entclient.SignupItemMutation) (ent.Value, error) {
	if !changesMark(m) {
		return next.Mutate(ctx, m)
	}
	ids, err := m.IDs(ctx)
	if err != nil {
		return nil, err
	}
	v, err := next.Mutate(ctx, m)
	if err != nil || len(ids) == 0 {
		return v, err
	}
	answered := user.HasConsentRecordsWith(consentrecord.HasVersionWith(legaldocumentversion.ItemIDIn(ids...)))
	return v, touch(ctx, m.Client(), answered)
}

// touch gives each User that which selects the next position, all of them the same one.
func touch(ctx context.Context, c *entclient.Client, which predicate.User) error {
	position, err := nextPosition(ctx, c)
	if err != nil {
		return err
	}
	if err := c.User.Update().Where(which).SetFeedPosition(position).Exec(ctx); err != nil {
		return fmt.Errorf("user feed position: %w", err)
	}
	return nil
}

// bury leaves a tombstone of each deleted User at the next position.
func bury(ctx context.Context, c *entclient.Client, userIDs []int64) error {
	position, err := nextPosition(ctx, c)
	if err != nil {
		return err
	}
	builders := make([]*entclient.UserTombstoneCreate, len(userIDs))
	for i, id := range userIDs {
		builders[i] = c.UserTombstone.Create().SetUserID(id).SetFeedPosition(position)
	}
	if err := c.UserTombstone.CreateBulk(builders...).Exec(ctx); err != nil {
		return fmt.Errorf("user feed tombstone: %w", err)
	}
	return nil
}

// nextPosition is the next position, past every User's and every tombstone's. The lock is taken first
// and held to the end of the surrounding transaction, so no other transaction can take the same
// position between this read and the write, and a later position never commits first. It is the
// Workspace feed's lock: one change (a sign-up, a User's deletion) moves both feeds, and two
// locks taken in either order would deadlock.
func nextPosition(ctx context.Context, c *entclient.Client) (int64, error) {
	if err := postgres.LockTx(ctx, c, workspacefeed.LockKey); err != nil {
		return 0, fmt.Errorf("user feed lock: %w", err)
	}
	users, err := c.User.Query().
		Order(entclient.Desc(user.FieldFeedPosition)).Limit(1).
		Select(user.FieldFeedPosition).Ints(ctx)
	if err != nil {
		return 0, fmt.Errorf("user feed position: %w", err)
	}
	tombstones, err := c.UserTombstone.Query().
		Order(entclient.Desc(usertombstone.FieldFeedPosition)).Limit(1).
		Select(usertombstone.FieldFeedPosition).Ints(ctx)
	if err != nil {
		return 0, fmt.Errorf("user feed position: %w", err)
	}
	top := max(lo.Max(users), lo.Max(tombstones))
	return int64(top) + 1, nil
}
