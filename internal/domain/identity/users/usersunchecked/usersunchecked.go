// Package usersunchecked holds the users module's unchecked removal (ADR 0110, ADR 0145): deleting a
// User first proves who asks, refuses the only Owner of a live Workspace and removes every
// Membership, which needs the memberships and the Second factors modules beside users, so those
// checks are the use case internal/usecase/userdeletion's (ADR 0050). Every rule of the User's own
// row stays here: the proof still fits it. Only a use case may import this package. The users
// module does not import it.
package usersunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// Deletion is the removal of one User, opened on its row under the row's lock so that the use case
// runs its checks (the Second factor code, the only Owner of a live Workspace) between opening and
// applying it, on the User the deletion locked.
type Deletion struct {
	tx   *ent.Client
	user *ent.User
}

// DeletionIn opens the deletion of the User proven, as read for the proof of who asks, on the
// transaction tx: it locks the User's row until tx ends, so a change of the User's password waits
// for it. A password set or removed since the proof was read means the proof no longer fits the
// User: the deletion is refused with users.ErrProofStale.
func DeletionIn(ctx context.Context, tx *ent.Client, proven *ent.User) (*Deletion, error) {
	locked, err := users.LockIn(ctx, tx, proven.ID)
	if err != nil {
		return nil, err
	}
	hadPassword := proven.PasswordHash != ""
	hasPassword := locked.PasswordHash != ""
	if hasPassword != hadPassword {
		return nil, &users.DeletionRefusedError{Reason: users.ErrProofStale}
	}
	return &Deletion{tx: tx, user: locked}, nil
}

// User is the User as the deletion locked it.
func (d *Deletion) User() *ent.User { return d.user }

// Apply deletes the User and publishes events.UserDeleted, which tells the address (ADR 0105),
// without asking who asks or what the User owns: the use case proved the one and refused the only
// Owner of a live Workspace, and removed every Membership, under the same lock before. Linked
// identities, the Second factor and its Recovery codes follow by cascade, an Invitation the User
// sent keeps its place with no sender, and Audit entries that name the User are never rewritten.
// Sessions need nothing: the security handler finds no User.
func (d *Deletion) Apply(ctx context.Context, pub events.Publisher) error {
	if err := d.tx.User.DeleteOneID(d.user.ID).Exec(ctx); err != nil {
		return err
	}
	return pub.Publish(ctx, &events.UserDeleted{UserID: d.user.ID, Email: d.user.Email})
}
