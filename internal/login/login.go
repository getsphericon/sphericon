// Package login is the first step of signing in (ADR 0020), shared by every surface
// that signs a person in with an address and a password: it normalizes the address
// once, consults the throttle (ADR 0025), looks the account up and checks the password
// against a decoy hash when there is none, applies the verification gate (ADR 0030),
// counts a failure and mints the second-factor challenge. It returns one Result and
// holds no database client: an adapter supplies the Store, the throttle's counter, the
// challenge and, after an Authenticated result, the session.
//
// The order is the module's contract, and is asserted without HTTP:
//
//  1. the throttle is consulted first, so a correct password is refused while the
//     address's delay runs (else guessing until one attempt succeeds would skip it);
//  2. the password is always hashed, against a decoy for an unknown address, so the
//     answer's time does not say which addresses exist;
//  3. the gate precedes the failure count and the challenge, and counts nothing;
//  4. an error from counting a failure refuses the attempt (a broken counter never lets
//     guessing through);
//  5. the password step resets the counter only when no second factor follows: the
//     second step resets it, else knowing the password would buy a fresh round of code
//     guesses on every sign-in.
package login

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/credentials"
)

// Outcome is what the password step decided.
type Outcome int

const (
	// Throttled: the address's delay is running. Result.Wait, Limit and Now say how long.
	Throttled Outcome = iota + 1
	// InvalidCredentials: an unknown address, a wrong password or an account without a
	// password, indistinguishably. The failure is counted.
	InvalidCredentials
	// Gated: the verification gate refused the account. Nothing is counted and no
	// challenge is minted.
	Gated
	// SecondFactor: the password is right and a second step follows. Result.Challenge
	// is its token.
	SecondFactor
	// Authenticated: the password is right and nothing else is needed. The counter is
	// reset; the adapter issues the session.
	Authenticated
)

// Result is the one answer of the password step. Principal is set for Gated,
// SecondFactor and Authenticated.
type Result[P any] struct {
	Outcome   Outcome
	Principal P
	// Wait is how long the address must still wait, Limit the failure threshold and
	// Now the throttle's clock, for a Throttled result.
	Wait  time.Duration
	Limit int
	Now   time.Time
	// Challenge is the second step's token, for a SecondFactor result.
	Challenge string
}

// Throttle is the failure counter of one login surface (a User's, or a Staff member's:
// each has its own, so one cannot lock the other). Addresses reach it normalized.
type Throttle interface {
	Delay(ctx context.Context, address string) (time.Duration, error)
	RecordFailure(ctx context.Context, address string) error
	RecordSuccess(ctx context.Context, address string) error
	Limit() int
	Now() time.Time
}

// Account is what a Store knows about an address.
type Account[P any] struct {
	Principal P
	// PasswordHash is empty for an account without a password.
	PasswordHash string
	// SecondFactor reports that a second step follows a right password.
	SecondFactor bool
}

// Store looks an account up by its normalized address; ok is false when there is none.
type Store[P any] interface {
	Lookup(ctx context.Context, address string) (acc Account[P], ok bool, err error)
}

// Config wires the module to one surface.
type Config[P any] struct {
	Throttle Throttle
	Store    Store[P]
	// Gate, when set, reports that an account may not sign in yet (an Unverified User,
	// ADR 0030). It is consulted for every known account, before the password result is
	// acted on.
	Gate func(P) bool
	// Challenge mints the second step's token for an account with a second factor.
	Challenge func(ctx context.Context, principal P) (string, error)
}

// Module is the password step.
type Module[P any] struct{ cfg Config[P] }

// New builds the module over a surface's adapter.
func New[P any](cfg Config[P]) *Module[P] { return &Module[P]{cfg: cfg} }

// Password runs the first step for an address and a password as typed.
func (m *Module[P]) Password(ctx context.Context, address, password string) (Result[P], error) {
	address = accounts.NormalizeEmail(address)

	wait, err := m.cfg.Throttle.Delay(ctx, address)
	if err != nil {
		return Result[P]{}, err
	}
	if wait > 0 {
		return Result[P]{Outcome: Throttled, Wait: wait, Limit: m.cfg.Throttle.Limit(), Now: m.cfg.Throttle.Now()}, nil
	}

	acc, found, err := m.cfg.Store.Lookup(ctx, address)
	if err != nil {
		return Result[P]{}, err
	}
	ok := verify(acc.PasswordHash, password)

	if found && m.cfg.Gate != nil && m.cfg.Gate(acc.Principal) {
		return Result[P]{Outcome: Gated, Principal: acc.Principal}, nil
	}
	if !found || !ok {
		if err := m.cfg.Throttle.RecordFailure(ctx, address); err != nil {
			return Result[P]{}, err
		}
		return Result[P]{Outcome: InvalidCredentials}, nil
	}
	if acc.SecondFactor {
		challenge, err := m.cfg.Challenge(ctx, acc.Principal)
		if err != nil {
			return Result[P]{}, err
		}
		return Result[P]{Outcome: SecondFactor, Principal: acc.Principal, Challenge: challenge}, nil
	}
	if err := m.cfg.Throttle.RecordSuccess(ctx, address); err != nil {
		// The password is proven; a counter that did not reset must not cost the sign-in.
		slog.ErrorContext(ctx, "login: counter not reset", "error", err)
	}
	return Result[P]{Outcome: Authenticated, Principal: acc.Principal}, nil
}

// verify checks the password; an empty hash (no account, or no password) is checked
// against a throwaway hash so the work is the same.
func verify(hash, password string) bool {
	if hash == "" {
		credentials.VerifyPassword(decoyHash(), password)
		return false
	}
	return credentials.VerifyPassword(hash, password)
}

// decoyHash is a real password hash nobody knows the password of, hashed once.
var decoyHash = sync.OnceValue(func() string {
	h, err := credentials.HashPassword("decoy password: no account matches")
	if err != nil {
		return ""
	}
	return h
})
