// Package userlogin is the Login of a User (GLOSSARY "Login", ADR 0020): the login module
// (internal/domain/identity/login) over the User's adapter, as internal/domain/identity/staffmembers
// is over the Staff member's. It looks a User up by the address they sign in with and says whether
// a Second factor follows (users, secondfactors), applies the verification gate (ADR 0030) when the
// Instance requires it, mints and parses the second step's challenge, checks its code through the
// Second factor (a TOTP code, or a Recovery code spent once) and finds and verifies a Passkey login
// (passkeys, ADR 0052). A step that signs the User in records it as `user.login` with the method that
// began it (ADR 0022, 0048), so a surface only turns the outcome into a session.
//
// The order of each step is the login module's contract; this package adds no rule to it but the
// entry. It holds no client: the modules it composes own their rows.
package userlogin

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/login"
	"github.com/getsphericon/sphericon/internal/domain/identity/passkeys"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/kernel/authtoken"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
)

const (
	// methodClaim is the challenge claim that names how the first step proved the person.
	methodClaim = "method"
	// providerLoginClaim is the challenge claim that holds the time of the provider login it
	// follows.
	providerLoginClaim = "provider_login"
	// challengeTTL is how long a second step's challenge stays valid.
	challengeTTL = 5 * time.Minute
)

// Config is what the User's Login is built from.
type Config struct {
	Users         *users.Module
	SecondFactors *secondfactors.Module
	// Passkeys finds and verifies a Passkey login (ADR 0052).
	Passkeys *passkeys.Module
	// Throttle is the Users' Login throttle (throttle.KindLogin, ADR 0025).
	Throttle login.Throttle
	// Passwords is the hasher the stored passwords were made with.
	Passwords *credentials.Hasher
	// Challenges signs the second step's challenge.
	Challenges *authtoken.Signer
	// RequireVerification is the verification gate (ADR 0030): an Unverified User may not sign in.
	RequireVerification bool
}

// Module is the Login of a User.
type Module struct {
	users      *users.Module
	factors    *secondfactors.Module
	passkeys   *passkeys.Module
	challenges *authtoken.Signer
	login      *login.Module[*ent.User]
}

// New builds the Login of a User.
func New(cfg Config) *Module {
	m := &Module{users: cfg.Users, factors: cfg.SecondFactors, passkeys: cfg.Passkeys, challenges: cfg.Challenges}
	lc := login.Config[*ent.User]{
		Throttle:      cfg.Throttle,
		Store:         store{m},
		Passwords:     cfg.Passwords,
		Challenge:     m.mintChallenge,
		Parse:         m.parseChallenge,
		Verify:        m.verifyCode,
		PasskeyFind:   m.findPasskey,
		PasskeyVerify: m.verifyPasskey,
	}
	if cfg.RequireVerification {
		lc.Gate = func(u *ent.User) bool { return u.EmailVerifiedAt == nil }
	}
	m.login = login.New(lc)
	return m
}

// Password is the first step for an address and a password as typed (login.Module.Password). An
// Authenticated result is recorded as `user.login` by password.
func (m *Module) Password(ctx context.Context, address, password string) (login.Result[*ent.User], error) {
	res, err := m.login.Password(ctx, address, password)
	if err == nil && res.Outcome == login.Authenticated {
		m.recordLogin(ctx, res.Principal, users.LoginPassword)
	}
	return res, err
}

// SecondFactor is the second step for a challenge and a code as typed (login.Module.SecondFactor).
// Its result is recorded as `user.login` with the method the challenge carries:
// a password, or the Sign-in provider whose login it follows.
func (m *Module) SecondFactor(ctx context.Context, challenge, code string) (login.SecondResult[*ent.User], error) {
	res, err := m.login.SecondFactor(ctx, challenge, code)
	if err == nil {
		m.recordLogin(ctx, res.Principal, res.Method)
	}
	return res, err
}

// Passkey is the Passkey login for a response as the browser sent it (login.Module.Passkey). A
// result is recorded as `user.login` by Passkey.
func (m *Module) Passkey(ctx context.Context, a login.PasskeyAssertion) (login.PasskeyResult[*ent.User], error) {
	res, err := m.login.Passkey(ctx, a)
	if err == nil {
		m.recordLogin(ctx, res.Principal, users.LoginPasskey)
	}
	return res, err
}

// Challenge signs the second step's challenge of User u, bound to binding
// (secondfactors.Module.ChallengeBinding), so it stops verifying after one successful second step
// (or any epoch bump or password change), with no store. method (users.LoginPassword or a Sign-in
// provider's name) rides in the signed claims, so the second step records the login with the method
// that began it; a provider login also passes its time (zero for a password login), which the
// challenge carries to the session the second step starts (ADR 0048). It is how a sign-in through
// a Sign-in provider that finds a Second factor due hands its second step to this Login.
func (m *Module) Challenge(u *ent.User, binding, method string, providerLoginAt time.Time) (string, error) {
	extra := map[string]string{methodClaim: method}
	if !providerLoginAt.IsZero() {
		extra[providerLoginClaim] = strconv.FormatInt(providerLoginAt.Unix(), 10)
	}
	return m.challenges.Mint(authtoken.PurposeLoginChallenge, u.ID, binding, challengeTTL, extra)
}

// recordLogin records a sign-in as `user.login` (users.Module.RecordLogin). The person is proven
// when it is written, so an entry that fails is logged and does not refuse the sign-in.
func (m *Module) recordLogin(ctx context.Context, u *ent.User, method string) {
	if err := m.users.RecordLogin(ctx, u, method); err != nil {
		slog.ErrorContext(ctx, "audit: login not recorded", "error", err)
	}
}

// store is the User's side of the login module's seam: Users by the address they sign in with.
type store struct{ m *Module }

func (s store) Lookup(ctx context.Context, address string) (login.Account[*ent.User], bool, error) {
	u, err := s.m.users.UserByLoginAddress(ctx, address)
	if ent.IsNotFound(err) {
		return login.Account[*ent.User]{}, false, nil
	}
	if err != nil {
		return login.Account[*ent.User]{}, false, err
	}
	active, err := s.m.factors.Active(ctx, u.ID)
	if err != nil {
		return login.Account[*ent.User]{}, false, err
	}
	return login.Account[*ent.User]{Principal: u, PasswordHash: u.PasswordHash, SecondFactor: active}, true, nil
}

// mintChallenge is the login module's Challenge: the challenge of a password login.
func (m *Module) mintChallenge(ctx context.Context, u *ent.User) (string, error) {
	binding, err := m.factors.ChallengeBinding(ctx, u)
	if err != nil {
		return "", err
	}
	return m.Challenge(u, binding, users.LoginPassword, time.Time{})
}

// parseChallenge is the login module's Parse: the Pending a still-valid challenge was minted for,
// its User, the binding it verified under and the method and provider login time it carries.
// login.ErrChallengeInvalid when it is forged, expired, already used, or its User is gone or no
// longer has a Second factor; other errors are the store's.
func (m *Module) parseChallenge(ctx context.Context, challenge string) (login.Pending[*ent.User], error) {
	var (
		u       *ent.User
		binding string
		loadErr error
	)
	_, extra, err := m.challenges.Parse(challenge, authtoken.PurposeLoginChallenge, func(id int64) (string, error) {
		if u, loadErr = m.users.User(ctx, id); loadErr != nil {
			return "", loadErr
		}
		binding, loadErr = m.factors.ChallengeBinding(ctx, u)
		return binding, loadErr
	})
	if loadErr != nil && !ent.IsNotFound(loadErr) && !errors.Is(loadErr, secondfactors.ErrNotActive) {
		return login.Pending[*ent.User]{}, loadErr
	}
	if err != nil {
		return login.Pending[*ent.User]{}, login.ErrChallengeInvalid
	}
	var providerLoginAt time.Time
	if sec, perr := strconv.ParseInt(extra[providerLoginClaim], 10, 64); perr == nil {
		providerLoginAt = time.Unix(sec, 0)
	}
	return login.Pending[*ent.User]{
		Principal:       u,
		Address:         u.Email,
		Binding:         binding,
		Method:          cmp.Or(extra[methodClaim], users.LoginPassword),
		ProviderLoginAt: providerLoginAt,
	}, nil
}

// verifyCode is the login module's Verify: the Second factor's check of a code (a TOTP code, or a
// Recovery code spent once) under the binding the challenge was minted with.
func (m *Module) verifyCode(ctx context.Context, p login.Pending[*ent.User], code string) error {
	_, err := m.factors.Verify(ctx, p, code)
	switch {
	case errors.Is(err, secondfactors.ErrInvalidCode):
		return login.ErrInvalidCode
	case errors.Is(err, secondfactors.ErrNotActive), errors.Is(err, secondfactors.ErrBindingMoved):
		// The factor is gone, or a racing second step spent the challenge first.
		return login.ErrChallengeInvalid
	}
	return err
}

// findPasskey is the login module's PasskeyFind: the User the response's credential belongs to.
func (m *Module) findPasskey(ctx context.Context, a login.PasskeyAssertion) (login.Pending[*ent.User], error) {
	u, err := m.passkeys.FindLogin(ctx, a.Response)
	if errors.Is(err, passkeys.ErrPasskeyUnknown) || errors.Is(err, passkeys.ErrPasskeysUnavailable) {
		return login.Pending[*ent.User]{}, login.ErrPasskeyUnknown
	}
	if err != nil {
		return login.Pending[*ent.User]{}, err
	}
	return login.Pending[*ent.User]{Principal: u, Address: u.Email, Method: users.LoginPasskey}, nil
}

// verifyPasskey is the login module's PasskeyVerify.
func (m *Module) verifyPasskey(ctx context.Context, p login.Pending[*ent.User], a login.PasskeyAssertion) error {
	err := m.passkeys.FinishLogin(ctx, p.Principal, a.Ceremony, a.Response)
	if passkeys.CeremonyFailed(err) || errors.Is(err, passkeys.ErrPasskeysUnavailable) {
		return login.ErrPasskeyInvalid
	}
	return err
}
