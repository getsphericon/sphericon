// Package usersunchecked holds the users module's unchecked removal (ADR 0110): deleting a User
// first proves who asks, refuses the only Owner of a live Workspace and removes every Membership,
// which needs the memberships and the Second factors modules beside users, so the check is the use
// case internal/usecase/userdeletion's (ADR 0050); only a use case may import this package. The
// users module does not import it.
package usersunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// DeleteIn deletes the User u on the transaction tx and publishes events.UserDeleted, which tells
// the address (ADR 0105): it checks nothing. The use case has proven who asks, refused the only
// Owner of a live Workspace and removed every Membership first. Linked identities, the Second
// factor and its Recovery codes follow by cascade, an Invitation the User sent keeps its place with
// no sender, and Audit entries that name the User are never rewritten. Sessions need nothing: the
// security handler finds no User.
func DeleteIn(ctx context.Context, tx *ent.Client, pub events.Publisher, u *ent.User) error {
	if err := tx.User.DeleteOneID(u.ID).Exec(ctx); err != nil {
		return err
	}
	return pub.Publish(ctx, &events.UserDeleted{UserID: u.ID, Email: u.Email})
}
