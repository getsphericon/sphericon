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
//     guesses on every sign-in;
//  6. a Passkey login (ADR 0052) finds the account by the credential, consults that
//     account's throttle before verifying, and counts no failure: it replaces both steps.
package login

import (
	"context"
	"errors"
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

// SecondOutcome is what the second step decided.
type SecondOutcome int

const (
	// SecondThrottled: the address's delay is running. SecondResult.Wait, Limit and Now
	// say how long.
	SecondThrottled SecondOutcome = iota + 1
	// SecondInvalidChallenge: the challenge is forged, expired, spent, moved or its
	// account is gone. It has no trusted address, so nothing is counted.
	SecondInvalidChallenge
	// SecondInvalidCode: the code is wrong. The failure is counted against the same
	// budget as a wrong password.
	SecondInvalidCode
	// SecondAuthenticated: the code is right. The counter is reset; the adapter issues
	// the session.
	SecondAuthenticated
)

var (
	// ErrChallengeInvalid is what an adapter's Parse and Verify return for a challenge
	// that no longer stands (forged, expired, spent, moved, factor gone).
	ErrChallengeInvalid = errors.New("login: challenge invalid")
	// ErrInvalidCode is what an adapter's Verify returns for a wrong code.
	ErrInvalidCode = errors.New("login: invalid code")
)

// PasskeyOutcome is what the Passkey step decided (ADR 0052).
type PasskeyOutcome int

const (
	// PasskeyThrottled: the delay of the User the credential names is running.
	PasskeyThrottled PasskeyOutcome = iota + 1
	// PasskeyFailed is every other refusal, indistinguishably: an unknown credential, a
	// response that does not verify (no user verification included), a spent or expired
	// challenge, an account the verification gate refuses. Nothing is counted.
	PasskeyFailed
	// PasskeyAuthenticated: the credential is proven and the person verified. The counter
	// is reset; the adapter issues the session. No Second factor follows.
	PasskeyAuthenticated
)

// ErrPasskeyUnknown is what an adapter's PasskeyFind returns for a response that names no
// credential (malformed, or a credential nobody registered).
var ErrPasskeyUnknown = errors.New("login: passkey unknown")

// ErrPasskeyInvalid is what an adapter's PasskeyVerify returns for a response that does not
// verify: a bad signature, no user verification, a spent or expired challenge.
var ErrPasskeyInvalid = errors.New("login: passkey response invalid")

// PasskeyAssertion is what the browser sent to finish a Passkey login: the ceremony cookie
// that carries the challenge and the authenticator's response.
type PasskeyAssertion struct {
	Ceremony string
	Response []byte
}

// PasskeyResult is the one answer of the Passkey step. Principal is set for
// PasskeyAuthenticated.
type PasskeyResult[P any] struct {
	// Delay is set for a PasskeyThrottled result.
	Delay

	Outcome   PasskeyOutcome
	Principal P
}

// Pending is a challenge the adapter parsed: the account it was minted for, the address
// that counts the second step's failures and what the session needs to know.
type Pending[P any] struct {
	Principal P
	// Address is the account's throttle key (normalized again by the module).
	Address string
	// Method names how the first step proved the person; ProviderLoginAt is the time of
	// a provider login the challenge follows (zero for a password login).
	Method          string
	ProviderLoginAt time.Time
	// Binding is the adapter's own single-use proof, handed back to Verify.
	Binding string
}

// Delay is the throttle's refusal: how long the address must still wait, the failure
// threshold and the throttle's clock. A throttled result of any step carries it.
type Delay struct {
	Wait  time.Duration
	Limit int
	Now   time.Time
}

// Result is the one answer of the password step. Principal is set for Gated,
// SecondFactor and Authenticated.
type Result[P any] struct {
	// Delay is set for a Throttled result.
	Delay

	Outcome   Outcome
	Principal P
	// Challenge is the second step's token, for a SecondFactor result.
	Challenge string
}

// SecondResult is the one answer of the second step. Principal, Method and
// ProviderLoginAt (the last two come from the challenge) are set for SecondAuthenticated.
type SecondResult[P any] struct {
	// Delay is set for a SecondThrottled result.
	Delay

	Outcome         SecondOutcome
	Principal       P
	Method          string
	ProviderLoginAt time.Time
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
	// Passwords hashes the decoy a password is checked against when no account matches: the
	// hasher the stored passwords were made with, so the check costs the same.
	Passwords *credentials.Hasher
	// Gate, when set, reports that an account may not sign in yet (an Unverified User,
	// ADR 0030). It is consulted for every known account, before the password result is
	// acted on.
	Gate func(P) bool
	// Challenge mints the second step's token for an account with a second factor.
	Challenge func(ctx context.Context, principal P) (string, error)
	// Parse turns a challenge token into its Pending, or returns ErrChallengeInvalid.
	Parse func(ctx context.Context, token string) (Pending[P], error)
	// Verify checks a code for a Pending under the adapter's lock (a TOTP code, or a
	// recovery code, which it spends). It returns ErrInvalidCode for a wrong code and
	// ErrChallengeInvalid when the challenge stopped standing (the factor is gone, or a
	// racing second step moved it first).
	Verify func(ctx context.Context, p Pending[P], code string) error
	// PasskeyFind names the account (Pending.Principal and the throttle key Pending.Address)
	// a Passkey response belongs to, by its credential id, or returns ErrPasskeyUnknown. It
	// verifies nothing. Left nil, the surface has no Passkey login.
	PasskeyFind func(ctx context.Context, a PasskeyAssertion) (Pending[P], error)
	// PasskeyVerify checks the response for the account PasskeyFind named: signature,
	// challenge, user verification, single use. It returns ErrPasskeyInvalid when it does not
	// hold.
	PasskeyVerify func(ctx context.Context, p Pending[P], a PasskeyAssertion) error
}

// Module is the password step.
type Module[P any] struct {
	cfg       Config[P]
	decoyOnce sync.Once
	decoy     string
}

// New builds the module over a surface's adapter.
func New[P any](cfg Config[P]) *Module[P] { return &Module[P]{cfg: cfg} }

// throttle consults the throttle for a normalized address; wait is true, with the Delay
// to report, while the address must wait whatever the password or code.
func (m *Module[P]) throttle(ctx context.Context, address string) (delay Delay, wait bool, err error) {
	d, err := m.cfg.Throttle.Delay(ctx, address)
	if err != nil || d <= 0 {
		return Delay{}, false, err
	}
	return Delay{Wait: d, Limit: m.cfg.Throttle.Limit(), Now: m.cfg.Throttle.Now()}, true, nil
}

// Password runs the first step for an address and a password as typed.
func (m *Module[P]) Password(ctx context.Context, address, password string) (Result[P], error) {
	address = accounts.NormalizeEmail(address)

	delay, wait, err := m.throttle(ctx, address)
	if err != nil {
		return Result[P]{}, err
	}
	if wait {
		return Result[P]{Outcome: Throttled, Delay: delay}, nil
	}

	acc, found, err := m.cfg.Store.Lookup(ctx, address)
	if err != nil {
		return Result[P]{}, err
	}
	ok := m.verify(acc.PasswordHash, password)

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

// SecondFactor runs the second step for a challenge and a code as typed. Order: a bad
// challenge is refused without counting; the throttle is consulted before the code, so
// a correct code is refused while the delay runs; a wrong code counts against the same
// budget as a wrong password (a counting error refuses the attempt); a committed second
// factor resets the counter, and a reset that fails is logged, not fatal.
func (m *Module[P]) SecondFactor(ctx context.Context, challenge, code string) (SecondResult[P], error) {
	p, err := m.cfg.Parse(ctx, challenge)
	if errors.Is(err, ErrChallengeInvalid) {
		return SecondResult[P]{Outcome: SecondInvalidChallenge}, nil
	}
	if err != nil {
		return SecondResult[P]{}, err
	}
	address := accounts.NormalizeEmail(p.Address)

	delay, wait, err := m.throttle(ctx, address)
	if err != nil {
		return SecondResult[P]{}, err
	}
	if wait {
		return SecondResult[P]{Outcome: SecondThrottled, Delay: delay}, nil
	}

	switch err := m.cfg.Verify(ctx, p, code); {
	case errors.Is(err, ErrInvalidCode):
		if err := m.cfg.Throttle.RecordFailure(ctx, address); err != nil {
			return SecondResult[P]{}, err
		}
		return SecondResult[P]{Outcome: SecondInvalidCode}, nil
	case errors.Is(err, ErrChallengeInvalid):
		return SecondResult[P]{Outcome: SecondInvalidChallenge}, nil
	case err != nil:
		return SecondResult[P]{}, err
	}
	if err := m.cfg.Throttle.RecordSuccess(ctx, address); err != nil {
		// The second factor is committed (a code is spent); a counter that did not reset
		// must not cost the sign-in.
		slog.ErrorContext(ctx, "login: counter not reset", "error", err)
	}
	return SecondResult[P]{Outcome: SecondAuthenticated, Principal: p.Principal, Method: p.Method, ProviderLoginAt: p.ProviderLoginAt}, nil
}

// Passkey runs the Passkey login for a response as the browser sent it (ADR 0052). The
// account is found by the credential, never by an address. Order: an unknown credential is
// refused without counting; the throttle of the account found is consulted before the
// response is verified, so a correct response is refused while the delay runs; the
// verification gate refuses an account that may not sign in yet, with the same answer and no
// count; a response that does not verify is the same refusal and counts nothing (without the
// private key a guess cannot succeed); a verified response resets the counter, and a reset
// that fails is logged, not fatal. A Passkey replaces the password and the Second factor, so
// no second step follows.
func (m *Module[P]) Passkey(ctx context.Context, a PasskeyAssertion) (PasskeyResult[P], error) {
	p, err := m.cfg.PasskeyFind(ctx, a)
	if errors.Is(err, ErrPasskeyUnknown) {
		return PasskeyResult[P]{Outcome: PasskeyFailed}, nil
	}
	if err != nil {
		return PasskeyResult[P]{}, err
	}
	address := accounts.NormalizeEmail(p.Address)

	delay, wait, err := m.throttle(ctx, address)
	if err != nil {
		return PasskeyResult[P]{}, err
	}
	if wait {
		return PasskeyResult[P]{Outcome: PasskeyThrottled, Delay: delay}, nil
	}
	if m.cfg.Gate != nil && m.cfg.Gate(p.Principal) {
		return PasskeyResult[P]{Outcome: PasskeyFailed}, nil
	}

	err = m.cfg.PasskeyVerify(ctx, p, a)
	if errors.Is(err, ErrPasskeyInvalid) {
		return PasskeyResult[P]{Outcome: PasskeyFailed}, nil
	}
	if err != nil {
		return PasskeyResult[P]{}, err
	}
	if err := m.cfg.Throttle.RecordSuccess(ctx, address); err != nil {
		slog.ErrorContext(ctx, "login: counter not reset", "error", err)
	}
	return PasskeyResult[P]{Outcome: PasskeyAuthenticated, Principal: p.Principal}, nil
}

// verify checks the password; an empty hash (no account, or no password) is checked
// against a throwaway hash of the same cost so the work is the same.
func (m *Module[P]) verify(hash, password string) bool {
	if hash == "" {
		credentials.VerifyPassword(m.decoyHash(), password)
		return false
	}
	return credentials.VerifyPassword(hash, password)
}

// decoyHash is a real password hash nobody knows the password of, hashed once.
func (m *Module[P]) decoyHash() string {
	m.decoyOnce.Do(func() {
		m.decoy, _ = m.cfg.Passwords.Hash("decoy password: no account matches")
	})
	return m.decoy
}
