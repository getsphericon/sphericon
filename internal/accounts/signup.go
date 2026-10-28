package accounts

import (
	"context"
	"errors"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/postgres"
)

// SignupRule is who may register on the Instance (ADR 0060). It follows from the Instance's Mode;
// an Invitation and `sphericon user create-owner` open a closed sign-up whatever the rule says.
type SignupRule int

const (
	// SignupAlways lets anyone who reaches the Instance register: the saas Mode.
	SignupAlways SignupRule = iota
	// SignupUntilFirstUser lets anyone register until the Instance has a User, the first Owner,
	// and closes sign-up after that: the selfhosted Mode.
	SignupUntilFirstUser
)

// SignupRuleOf is the rule of a Mode: saas is SignupAlways, selfhosted SignupUntilFirstUser.
func SignupRuleOf(saas bool) SignupRule {
	if saas {
		return SignupAlways
	}
	return SignupUntilFirstUser
}

// ErrSignupClosed is a registration the Instance's sign-up rule refuses.
var ErrSignupClosed = errors.New("accounts: sign-up is closed")

// signupLockKey is the advisory lock that makes two first registrations of a fresh Instance take
// turns, so only the first finds the Instance empty.
const signupLockKey = 7_204_002

// WithSignupRule sets who may register; the default is SignupAlways.
func WithSignupRule(rule SignupRule) Option {
	return func(a *Accounts) { a.signup = rule }
}

// SignupOpen reports whether a person with no Invitation may register now. It is the cheap answer
// that refuses early and feeds the sign-in decision; the transaction that creates the User decides
// again (permitSignup), because another registration may have come in between.
func (a *Accounts) SignupOpen(ctx context.Context) (bool, error) {
	if a.signup == SignupAlways {
		return true, nil
	}
	hasUser, err := a.ent.User.Query().Exist(ctx)
	return !hasUser, err
}

// permitSignup is the sign-up rule inside the transaction that creates a User. Under
// SignupUntilFirstUser it first takes the advisory lock to the end of the transaction, so a second
// first registration waits and then sees the first one's User.
func (a *Accounts) permitSignup(ctx context.Context, tx *ent.Client) error {
	if a.signup == SignupAlways {
		return nil
	}
	if err := postgres.LockTx(ctx, tx, signupLockKey); err != nil {
		return err
	}
	hasUser, err := tx.User.Query().Exist(ctx)
	if err != nil {
		return err
	}
	if hasUser {
		return ErrSignupClosed
	}
	return nil
}
