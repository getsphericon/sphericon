// Package emailcase keeps stored addresses lower-cased: the email of a User, an Invitation and a
// Contact, so one address is one row whatever case it was typed in, the unique indexes on
// users.email and on a Workspace's contacts.email hold case-insensitively, an Invitation matches
// the User it names, and a Contact's address matches the Suppressions and Unsubscribes of the same
// Destination, which are stored lower-cased too.
//
// Hook lower-cases the email of every create and update of those three, so no write path can
// forget it. It runs on every client (db.NewEntClient, db.NewEntClientOnTx and the event bus's
// transaction client; a client from Tx inherits it). The CHECK constraints <table>_email_lower
// (ent/schema LowerEmailCheck) refuse a write that went around it, so a missed path fails
// loudly instead of making a second account.
package emailcase

import (
	"context"
	"strings"

	"entgo.io/ent"

	entclient "github.com/getsphericon/sphericon/ent"
)

// Hook lower-cases the email field of a User, Invitation or Contact mutation that sets it.
func Hook(next ent.Mutator) ent.Mutator {
	return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
		switch mu := m.(type) {
		case *entclient.UserMutation:
			if email, ok := mu.Email(); ok {
				mu.SetEmail(strings.ToLower(email))
			}
		case *entclient.InvitationMutation:
			if email, ok := mu.Email(); ok {
				mu.SetEmail(strings.ToLower(email))
			}
		case *entclient.ContactMutation:
			if email, ok := mu.Email(); ok {
				mu.SetEmail(strings.ToLower(email))
			}
		}
		return next.Mutate(ctx, m)
	})
}
