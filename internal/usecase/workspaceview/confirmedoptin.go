package workspaceview

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces/workspacesunchecked"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// SetConfirmedOptInInput turns confirmed opt-in On or off, made on Revision (ADR 0119).
type SetConfirmedOptInInput struct {
	Revision int64
	On       bool
}

// SetConfirmedOptIn turns confirmed opt-in of the Workspace of s on or off (ADR 0013) and answers
// its View for the caller. Only a person does it, in the console: turning it on asserts that the
// Workspace's current mailable list consented, so it records a grandfathered Confirmation for every
// address mailable now (confirmations.Module.Grandfather) before the policy turns on. The bulk of
// them goes in pages, each in a transaction of its own; then one transaction sweeps the list again
// from its first Contact, which finds what became mailable behind the pages (an import committed
// out of id order, a lifted opt-out), and switches the policy with its one Audit entry carrying
// their number. A Contact committed after that sweep reads the list and before it commits is the
// one left unconfirmed. It is refused (workspaces.ConfirmedOptInSwitch) on a revision the
// Workspace has moved past and, turning it on, while the confirmation email is not set up, before
// anything is recorded. A switch to the state the Workspace is in changes nothing: it does not
// grandfather the addresses that came since. Turning it off keeps every Confirmation.
func (u *UseCase) SetConfirmedOptIn(ctx context.Context, s *ent.Scoped, c Caller, in SetConfirmedOptInInput) (View, error) {
	ws, err := u.workspaces.Get(ctx, s)
	if err != nil {
		return View{}, err
	}
	changes, err := workspaces.ConfirmedOptInSwitch(ws, in.Revision, in.On)
	switch {
	case err != nil:
		return View{}, err
	case !changes:
		return u.view(ctx, s, c, ws)
	}
	recorded := 0
	if in.On {
		if recorded, err = u.grandfatherInPages(ctx, s); err != nil {
			return View{}, err
		}
	}
	err = u.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		if in.On {
			swept, err := u.grandfather(ctx, ts)
			if err != nil {
				return err
			}
			recorded += swept
		}
		switched, err := u.unchecked.SwitchConfirmedOptInIn(ctx, ts, workspacesunchecked.SwitchConfirmedOptInInput{
			Revision: in.Revision, On: in.On, Recorded: recorded,
		})
		if err != nil {
			return err
		}
		ws = switched.Workspace
		if switched.Audit == nil {
			return nil
		}
		return u.bus.RecordAuditIn(ctx, ts, switched.Audit)
	})
	if err != nil {
		return View{}, err
	}
	return u.view(ctx, s, c, ws)
}

// grandfatherInPages grandfathers the Workspace's mailable list a page at a time, each page in a
// transaction of its own, and returns how many Confirmations it recorded.
func (u *UseCase) grandfatherInPages(ctx context.Context, s *ent.Scoped) (int, error) {
	recorded := 0
	for page := (confirmations.Grandfathered{More: true}); page.More; {
		err := u.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
			var err error
			page, err = u.confirmations.Grandfather(ctx, ts, page.Last, confirmations.GrandfatherPage)
			return err
		})
		if err != nil {
			return 0, err
		}
		recorded += page.Recorded
	}
	return recorded, nil
}

// grandfather grandfathers the whole mailable list in the transaction ts is bound to, from its
// first Contact, and returns how many Confirmations it recorded.
func (u *UseCase) grandfather(ctx context.Context, ts *ent.Scoped) (int, error) {
	recorded := 0
	for page := (confirmations.Grandfathered{More: true}); page.More; {
		var err error
		if page, err = u.confirmations.Grandfather(ctx, ts, page.Last, confirmations.GrandfatherPage); err != nil {
			return 0, err
		}
		recorded += page.Recorded
	}
	return recorded, nil
}
