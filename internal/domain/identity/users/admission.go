package users

import (
	"context"

	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Issuer names the Instance to authenticator apps and Passkeys.
const Issuer = "sphericon"

// ErrAcceptancePending refuses a request on the own credential of a User with a required
// acceptance pending (ADR 0062). Every surface answers it 403 acceptance_pending.
var ErrAcceptancePending = validation.AcceptancePending("users: a required acceptance is pending", i18n.ErrorsAcceptancePending)

// CheckAcceptance is the User's part of the admission of a request on their own credential: nil
// unless the User has a required acceptance pending (ErrAcceptancePending). The admission as a
// whole, with the Two-factor requirement of a Workspace, is admission.Module.Admit.
func (a *Module) CheckAcceptance(ctx context.Context, userID int64) error {
	gated, err := a.consents.Gated(ctx, userID)
	if err != nil {
		return err
	}
	if gated {
		return ErrAcceptancePending
	}
	return nil
}
