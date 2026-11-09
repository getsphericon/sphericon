package login

import (
	"context"

	"github.com/getsphericon/sphericon/internal/infra/ratelimit"
	"github.com/getsphericon/sphericon/internal/infra/throttle"
)

// Proof is what proving the password of a signed-in account decided.
type Proof int

const (
	// ProofThrottled: the address's delay is running. ProofResult.Wait, Limit and Now say how long.
	ProofThrottled Proof = iota + 1
	// ProofHolds: the password is right.
	ProofHolds
)

// ProofResult is the one answer of Prove.
type ProofResult struct {
	// Delay is set for ProofThrottled.
	Delay

	Proof Proof
}

// Prove is the one path every password-gated action takes (second-factor enrol, disable
// and regenerate, a profile password or email change): the same throttle as the first
// step (consulted first, so a right password is refused while the delay runs), the same
// decoy for an account without a password, and a failure counted on the same budget as a
// login failure. The caller holds the account, so it passes its address and hash. A password
// that is not the account's is refused with ErrPasswordIncorrect.
func (m *Module[P]) Prove(ctx context.Context, address, passwordHash, password string) (ProofResult, error) {
	address = throttle.NormalizeEmail(address)

	delay, wait, err := m.throttle(ctx, address)
	if err != nil {
		return ProofResult{}, err
	}
	if wait {
		return ProofResult{Proof: ProofThrottled, Delay: delay}, nil
	}
	if m.verify(passwordHash, password) {
		return ProofResult{Proof: ProofHolds}, nil
	}
	if err := m.cfg.Throttle.RecordFailure(ctx, address); err != nil {
		return ProofResult{}, err
	}
	return ProofResult{}, ErrPasswordIncorrect
}

// ProvePassword is Prove for a step of a signed-in account that answers a running delay as a rate
// limit (users.Prover): nil when password is the account's, ErrPasswordIncorrect when it is not,
// and while the address's delay runs a *ratelimit.WaitError of the per-account login policy, even
// for a right password, which every surface answers as the password step's 429.
func (m *Module[P]) ProvePassword(ctx context.Context, address, passwordHash, password string) error {
	res, err := m.Prove(ctx, address, passwordHash, password)
	if err != nil {
		return err
	}
	if res.Proof == ProofThrottled {
		return &ratelimit.WaitError{Wait: res.Wait, Limit: res.Limit, Policy: ratelimit.PolicyLoginAccount, At: res.Now}
	}
	return nil
}

// Throttled consults the address's delay without proving anything, for a profile action
// whose proof is not the password (a mailed link) but which then takes a second-step code
// the caller counts with CountFailure: while the delay runs a code is not even checked, so
// the code cannot be guessed past the budget. It is the same throttle Prove consults first.
func (m *Module[P]) Throttled(ctx context.Context, address string) (delay Delay, throttled bool, err error) {
	return m.throttle(ctx, throttle.NormalizeEmail(address))
}

// CountFailure counts one wrong second-step code of an already proven password on the
// address's budget, the same budget a wrong password feeds (a profile action that asks for
// the password and then a code, such as User deletion).
func (m *Module[P]) CountFailure(ctx context.Context, address string) error {
	return m.cfg.Throttle.RecordFailure(ctx, throttle.NormalizeEmail(address))
}
