package login

import (
	"context"
	"time"

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
	// Wait, Limit and Now are set for ProofThrottled, as in Result.
	Wait  time.Duration
	Limit int
	Now   time.Time
}

// Prove is the one path every password-gated action takes (second-factor enrol, disable
// and regenerate, a profile password or email change): the same throttle as the first
// step (consulted first, so a right password is refused while the delay runs), the same
// decoy for an account without a password, and a failure counted on the same budget as a
// login failure. The caller holds the account, so it passes its address and hash.
func (m *Module[P]) Prove(ctx context.Context, address, passwordHash, password string) (ProofResult, error) {
	address = accounts.NormalizeEmail(address)

	wait, err := m.cfg.Throttle.Delay(ctx, address)
	if err != nil {
		return ProofResult{}, err
	}
	if wait > 0 {
		return ProofResult{Proof: ProofThrottled, Wait: wait, Limit: m.cfg.Throttle.Limit(), Now: m.cfg.Throttle.Now()}, nil
	}
	if verify(passwordHash, password) {
		return ProofResult{Proof: ProofHolds}, nil
	}
	if err := m.cfg.Throttle.RecordFailure(ctx, address); err != nil {
		return ProofResult{}, err
	}
	return ProofResult{Proof: ProofWrong}, nil
}
