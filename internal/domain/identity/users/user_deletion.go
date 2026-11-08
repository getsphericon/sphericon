package users

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The rules that refuse a User deletion (ADR 0050). A deletion answers each as a
// *DeletionRefusedError, whose answer names the Workspaces the User solely owns.
var (
	// ErrDeletionEmailMismatch: the email typed to confirm is not the User's own.
	ErrDeletionEmailMismatch = validation.RefuseField("email", "users: the typed email is not the user's", i18n.ErrorsDeleteUserEmailMismatch)
	// ErrDeletionUsePassword: a deletion link asked for a User who has a password, which
	// confirms the deletion instead.
	ErrDeletionUsePassword = validation.Refuse("users: the user confirms the deletion with the password", i18n.ErrorsDeleteUserUsePassword)
	// ErrProofStale: the proof no longer fits the User: the User's password was set or removed
	// between the proof and the transaction that locked the row.
	ErrProofStale = validation.Refuse("users: the deletion proof no longer fits the user", i18n.ErrorsDeleteUserProofStale)
	// ErrSoleOwner: the User is the only Owner of a live Workspace, the rule of any Owner change.
	ErrSoleOwner = validation.Refuse("users: the user is the only owner of live workspaces", i18n.ErrorsDeleteUserOnlyOwner)
)

// DeletionRefusedError is a User deletion refused by Reason, one of the rules above: nothing is
// deleted. For ErrSoleOwner, Workspaces says which Workspaces to hand over or delete first.
// errors.Is matches Reason.
type DeletionRefusedError struct {
	Reason *validation.RefusalError
	// Workspaces are the live Workspaces the User solely owns, in ascending id order.
	Workspaces []*ent.Workspace
}

func (e *DeletionRefusedError) Error() string {
	if len(e.Workspaces) == 0 {
		return e.Reason.Error()
	}
	names := lo.Map(e.Workspaces, func(w *ent.Workspace, _ int) string { return w.Name })
	return fmt.Sprintf("%s: %s", e.Reason.Error(), strings.Join(names, ", "))
}

// Unwrap is the rule that refused the deletion.
func (e *DeletionRefusedError) Unwrap() error { return e.Reason }

// refuseDeletion is the deletion refused by reason.
func refuseDeletion(reason *validation.RefusalError) error {
	return &DeletionRefusedError{Reason: reason}
}

// DeletionSubject is the User userID who asks to delete themself, once typedEmail, the email
// they typed to confirm, is theirs (ErrDeletionEmailMismatch otherwise).
func (a *Module) DeletionSubject(ctx context.Context, userID int64, typedEmail string) (*ent.User, error) {
	u, err := a.User(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(typedEmail), u.Email) {
		return nil, refuseDeletion(ErrDeletionEmailMismatch)
	}
	return u, nil
}

// DeleteIn deletes the User u on the transaction tx and publishes events.UserDeleted, which tells
// the address (ADR 0105), for User deletion (ADR 0050, the use case internal/usecase/userdeletion):
// it checks nothing. The use case has proven who asks, refused the only Owner of a live Workspace
// and removed every Membership first. Linked identities, the Second factor and its Recovery codes
// follow by cascade, an Invitation the User sent keeps its place with no sender, and Audit
// entries that name the User are never rewritten. Sessions need nothing: the security handler
// finds no User.
func DeleteIn(ctx context.Context, tx *ent.Client, pub events.Publisher, u *ent.User) error {
	if err := tx.User.DeleteOneID(u.ID).Exec(ctx); err != nil {
		return err
	}
	return pub.Publish(ctx, &events.UserDeleted{UserID: u.ID, Email: u.Email})
}
