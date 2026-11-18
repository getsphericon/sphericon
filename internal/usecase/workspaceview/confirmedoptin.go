package workspaceview

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
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
// address mailable now (confirmations.Module.Grandfather) before the policy turns on. The
// Confirmations go in pages, each in a transaction of its own; the last page, the policy and its
// one Audit entry with their number share one, so no address mailable then is left unconfirmed
// and a Broadcast in flight skips nobody. It is refused with ErrConfirmationEmailRequired while the
// Workspace has no confirmation email set up, and with ErrRevisionChanged on a revision it has moved
// past, both before anything is recorded. Turning it on while it is on, or off while it is off,
// changes nothing: it does not grandfather the addresses that came since. Turning it off keeps
// every Confirmation.
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
	var (
		page     confirmations.Grandfathered
		recorded int
	)
	for {
		err := u.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
			if in.On {
				var err error
				if page, err = u.confirmations.Grandfather(ctx, ts, page.Last, confirmations.GrandfatherPage); err != nil {
					return err
				}
				recorded += page.Recorded
				if page.More {
					return nil
				}
			}
			ws, err = u.workspaces.SwitchConfirmedOptInIn(ctx, ts, workspaces.SwitchConfirmedOptInInput{
				Revision: in.Revision, On: in.On, Recorded: recorded,
			})
			return err
		})
		if err != nil {
			return View{}, err
		}
		if !in.On || !page.More {
			return u.view(ctx, s, c, ws)
		}
	}
}
