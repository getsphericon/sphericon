package login

import (
	"context"

	"github.com/getsphericon/sphericon/internal/infra/throttle"
)

// ProvePassword is the one path every password-gated action takes (second-factor enrol, disable
// and regenerate, a profile password or email change, User deletion): the same throttle as the
// first step (consulted first, so a right password is refused while the delay runs, with the
// *ratelimit.WaitError every surface answers as the password step's 429), the same decoy for an
// account without a password, and a failure counted on the same budget as a login failure. The
// caller holds the account (users.Prover), so it passes its address and hash. It is nil when
// password is the account's, and ErrPasswordIncorrect when it is not.
func (m *Module[P]) ProvePassword(ctx context.Context, address, passwordHash, password string) error {
	address = throttle.NormalizeEmail(address)

	if err := m.throttle(ctx, address); err != nil {
		return err
	}
	if m.verify(passwordHash, password) {
		return nil
	}
	if err := m.cfg.Throttle.RecordFailure(ctx, address); err != nil {
		return err
	}
	return ErrPasswordIncorrect
}

// CheckThrottle consults the address's delay without proving anything, for a profile action
// whose proof is not the password (a mailed link) but which then takes a second-step code the
// caller counts with CountFailure: while the delay runs it is the *ratelimit.WaitError of the
// per-account login policy and a code is not even checked, so the code cannot be guessed past the
// budget. It is the same throttle ProvePassword consults first.
func (m *Module[P]) CheckThrottle(ctx context.Context, address string) error {
	return m.throttle(ctx, throttle.NormalizeEmail(address))
}

// CountFailure counts one wrong second-step code of an already proven password on the
// address's budget, the same budget a wrong password feeds (a profile action that asks for
// the password and then a code, such as User deletion).
func (m *Module[P]) CountFailure(ctx context.Context, address string) error {
	return m.cfg.Throttle.RecordFailure(ctx, throttle.NormalizeEmail(address))
}
