package accounts

import (
	"context"
	"errors"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/accounts/roles"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/ownernotice"
)

// The Owners of a Workspace (GLOSSARY "Role") are defined here and nowhere else: every
// lookup, lock and notice that needs them goes through these functions.

// Owners loads the owner Memberships of the Workspace s with their Users, oldest first.
func Owners(ctx context.Context, s *ent.Scoped) ([]*ent.Membership, error) {
	return s.Membership().Query().
		Where(roles.Owners()).
		WithUser().
		Order(ent.Asc(membership.FieldID)).
		All(ctx)
}

// OwnersOf loads the owner Memberships, with their Users, of every Workspace in workspaceIDs,
// oldest first. It is the paged form of Owners for callers that hold no scope (a list over
// many Workspaces).
func OwnersOf(ctx context.Context, client *ent.Client, workspaceIDs ...int64) ([]*ent.Membership, error) {
	return client.Membership.Query().
		Where(membership.WorkspaceIDIn(workspaceIDs...), roles.Owners()).
		WithUser().
		Order(ent.Asc(membership.FieldID)).
		All(ctx)
}

// LockOwners locks the owner Memberships of the Workspace s for the transaction and returns
// their ids. Locking first serializes concurrent Owner changes: the second waits, then reads
// the committed outcome of the first.
func LockOwners(ctx context.Context, s *ent.Scoped) ([]int64, error) {
	return s.Membership().Query().Where(roles.Owners()).ForUpdate().IDs(ctx)
}

// NotifyOwners sends one platform email to every Owner of the Workspace through the system
// sender, never the Workspace's own Integration (a suspended Workspace cannot send). It tries
// every Owner and joins the failures.
func NotifyOwners(ctx context.Context, client *ent.Client, sender messaging.EmailSender, workspaceID int64, subject, body string) error {
	owners, err := Owners(ctx, client.Scoped(workspaceID))
	if err != nil {
		return err
	}
	var errs []error
	for _, m := range owners {
		u := m.Edges.User
		if u == nil || u.Email == "" {
			continue
		}
		if _, serr := sender.Send(ctx, messaging.EmailMessage{To: u.Email, Subject: subject, Text: body}); serr != nil {
			errs = append(errs, fmt.Errorf("notify %s: %w", u.Email, serr))
		}
	}
	return errors.Join(errs...)
}

// OwnerNotifier is the ownernotice.Notify that mails a Workspace's Owners through the system sender
// (a nil sender gives a nil Notify, which sends nothing). It is the one seam of the modules that
// tell Owners something (Join domains, Sending domains, webhooks, suspension).
func OwnerNotifier(client *ent.Client, sender messaging.EmailSender) ownernotice.Notify {
	if sender == nil {
		return nil
	}
	return func(ctx context.Context, workspaceID int64, subject, body string) error {
		return NotifyOwners(ctx, client, sender, workspaceID, subject, body)
	}
}
