package login

import (
	"context"

	"github.com/getsphericon/sphericon/internal/accounts"
)

// Proof is what proving the password of a signed-in account decided.
type Proof int

const (
	// ProofThrottled: the address's delay is running. ProofResult.Wait, Limit and Now say how long.
	ProofThrottled Proof = iota + 1
	// ProofWrong: the password does not match (or the account has none). The failure is
	// counted on the same budget as a login failure.
	ProofWrong
	// ProofHolds: the password is right.
	ProofHolds
)

// ProofResult is the one answer of Prove.
type ProofResult struct {
	Proof Proof
	// Delay is set for ProofThrottled.
	Delay
}

// Prove is the one path every password-gated action takes (second-factor enrol, disable
// and regenerate, a profile password or email change): the same throttle as the first
// step (consulted first, so a right password is refused while the delay runs), the same
// decoy for an account without a password, and a failure counted on the same budget as a
// login failure. The caller holds the account, so it passes its address and hash.
func (m *Module[P]) Prove(ctx context.Context, address, passwordHash, password string) (ProofResult, error) {
	address = accounts.NormalizeEmail(address)

	delay, wait, err := m.throttle(ctx, address)
	if err != nil {
		return ProofResult{}, err
	}
	if wait {
		return ProofResult{Proof: ProofThrottled, Delay: delay}, nil
	}
	if verify(passwordHash, password) {
		return ProofResult{Proof: ProofHolds}, nil
	}
	if err := m.cfg.Throttle.RecordFailure(ctx, address); err != nil {
		return ProofResult{}, err
	}
	return ProofResult{Proof: ProofWrong}, nil
}

// Throttled consults the address's delay without proving anything, for a profile action
// whose proof is not the password (a mailed link) but which then takes a second-step code
// the caller counts with CountFailure: while the delay runs a code is not even checked, so
// the code cannot be guessed past the budget. It is the same throttle Prove consults first.
func (m *Module[P]) Throttled(ctx context.Context, address string) (delay Delay, throttled bool, err error) {
	return m.throttle(ctx, accounts.NormalizeEmail(address))
}

// CountFailure counts one wrong second-step code of an already proven password on the
// address's budget, the same budget a wrong password feeds (a profile action that asks for
// the password and then a code, such as User deletion).
func (m *Module[P]) CountFailure(ctx context.Context, address string) error {
	return m.cfg.Throttle.RecordFailure(ctx, accounts.NormalizeEmail(address))
}
