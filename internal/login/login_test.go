package login_test

import (
	"context"
	"errors"
	"go/build"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/login"
	"github.com/getsphericon/sphericon/internal/secondfactor"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// store is the adapter's side of the seam over the fixtures' Users.
type store struct{ db *ent.Client }

func (s store) Lookup(ctx context.Context, address string) (login.Account[*ent.User], bool, error) {
	u, err := s.db.User.Query().Where(user.EmailEqualFold(address)).First(ctx)
	if ent.IsNotFound(err) {
		return login.Account[*ent.User]{}, false, nil
	}
	if err != nil {
		return login.Account[*ent.User]{}, false, err
	}
	return login.Account[*ent.User]{Principal: u, PasswordHash: u.PasswordHash, SecondFactor: secondfactor.Active(u)}, true, nil
}

// resetFailing is a throttle whose success reset is broken.
type resetFailing struct{ login.Throttle }

func (resetFailing) RecordSuccess(context.Context, string) error { return errCounter }

// factor is the adapter's second-step side: challenges are "challenge-<user id>", the
// code "123456" is right any number of times and "recovery" is spent on first use.
type factor struct {
	db        *ent.Client
	spent     bool
	moved     bool
	verifyHit int
}

func (f *factor) Parse(ctx context.Context, token string) (login.Pending[*ent.User], error) {
	id, ok := strings.CutPrefix(token, "challenge-")
	n, err := strconv.ParseInt(id, 10, 64)
	if !ok || err != nil {
		return login.Pending[*ent.User]{}, login.ErrChallengeInvalid
	}
	u, err := f.db.User.Get(ctx, n)
	if err != nil {
		return login.Pending[*ent.User]{}, login.ErrChallengeInvalid
	}
	return login.Pending[*ent.User]{Principal: u, Address: u.Email, Method: "password"}, nil
}

func (f *factor) Verify(_ context.Context, _ login.Pending[*ent.User], code string) error {
	f.verifyHit++
	switch {
	case f.moved:
		return login.ErrChallengeInvalid
	case code == "123456":
		return nil
	case code == "recovery" && !f.spent:
		f.spent = true
		return nil
	}
	return login.ErrInvalidCode
}

// failingCounter is a throttle whose failure counter is broken.
type failingCounter struct{ login.Throttle }

var errCounter = errors.New("counter down")

func (failingCounter) RecordFailure(context.Context, string) error { return errCounter }

// clock is a settable time source for the counter.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

type rig struct {
	db     *ent.Client
	clock  *clock
	factor *factor
	module *login.Module[*ent.User]
	login  login.Throttle
	staff  login.Throttle
}

// newRig builds the module over a real counter that throttles at two failures.
func newRig(t *testing.T, opts ...func(*login.Config[*ent.User])) rig {
	t.Helper()
	env := testhelper.Setup(t)
	clk := &clock{t: time.Now()}
	counters := accounts.NewAttempts(env.DB,
		accounts.WithClock(clk.now),
		accounts.WithRule(accounts.KindLogin, accounts.LoginRule(2)),
		accounts.WithRule(accounts.KindStaffLogin, accounts.LoginRule(2)))
	r := rig{db: env.DB, clock: clk, login: counters.For(accounts.KindLogin), staff: counters.For(accounts.KindStaffLogin)}
	cfg := login.Config[*ent.User]{
		Throttle:  r.login,
		Store:     store{env.DB},
		Passwords: testhelper.Passwords(),
		Challenge: func(_ context.Context, u *ent.User) (string, error) {
			return "challenge-" + strconv.FormatInt(u.ID, 10), nil
		},
	}
	for _, o := range opts {
		o(&cfg)
	}
	r.factor = &factor{db: env.DB}
	cfg.Parse, cfg.Verify = r.factor.Parse, r.factor.Verify
	r.module = login.New(cfg)
	return r
}

func (r rig) second(t *testing.T, challenge, code string) login.SecondResult[*ent.User] {
	t.Helper()
	res, err := r.module.SecondFactor(t.Context(), challenge, code)
	require.NoError(t, err)
	return res
}

func (r rig) password(t *testing.T, address, password string) login.Result[*ent.User] {
	t.Helper()
	res, err := r.module.Password(t.Context(), address, password)
	require.NoError(t, err)
	return res
}

func TestAKnownAddressWithItsPasswordIsAuthenticated(t *testing.T) {
	r := newRig(t)
	res := r.password(t, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword)
	assert.Equal(t, login.Authenticated, res.Outcome)
	assert.Equal(t, int64(fixtures.OwnerJohnID), res.Principal.ID)
}

func TestTheAddressIsNormalizedOnceForTheLookupAndTheCounter(t *testing.T) {
	r := newRig(t)
	assert.Equal(t, login.Authenticated, r.password(t, "  INFO@GetSphericon.com ", fixtures.OwnerJohnPassword).Outcome)

	// Two spellings of one address feed one counter.
	assert.Equal(t, login.InvalidCredentials, r.password(t, fixtures.OwnerJohnEmail, "wrong").Outcome)
	assert.Equal(t, login.InvalidCredentials, r.password(t, " INFO@getsphericon.COM", "wrong").Outcome)
	assert.Equal(t, login.Throttled, r.password(t, fixtures.OwnerJohnEmail, "wrong").Outcome)
}

func TestThrottlePrecedesThePasswordCheckEvenForACorrectPassword(t *testing.T) {
	r := newRig(t)
	for range 2 {
		require.Equal(t, login.InvalidCredentials, r.password(t, fixtures.OwnerJohnEmail, "wrong").Outcome)
	}
	res := r.password(t, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword)
	assert.Equal(t, login.Throttled, res.Outcome)
	assert.Positive(t, res.Wait)
	assert.Equal(t, 2, res.Limit)
}

func TestThePasswordStepWorksAgainOnceTheDelayHasPassed(t *testing.T) {
	r := newRig(t)
	for range 2 {
		require.Equal(t, login.InvalidCredentials, r.password(t, fixtures.OwnerJohnEmail, "wrong").Outcome)
	}
	require.Equal(t, login.Throttled, r.password(t, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword).Outcome)

	r.clock.t = r.clock.t.Add(2 * time.Second)

	assert.Equal(t, login.Authenticated, r.password(t, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword).Outcome)
}

func TestAnUnknownAddressFollowsTheSamePathAsAKnownOne(t *testing.T) {
	r := newRig(t)
	const unknown = "nobody@login.test"
	assert.Equal(t, login.InvalidCredentials, r.password(t, unknown, "anything").Outcome)
	assert.Equal(t, login.InvalidCredentials, r.password(t, unknown, "anything").Outcome)
	// Counted like a known address, so it is throttled like one.
	assert.Equal(t, login.Throttled, r.password(t, unknown, "anything").Outcome)
}

func TestThePasswordStepDoesNotResetTheCounterWhenASecondFactorFollows(t *testing.T) {
	r := newRig(t)
	require.Equal(t, login.InvalidCredentials, r.password(t, fixtures.SecondFactorSamEmail, "wrong").Outcome)

	res := r.password(t, fixtures.SecondFactorSamEmail, fixtures.SecondFactorSamPassword)
	require.Equal(t, login.SecondFactor, res.Outcome)
	assert.Equal(t, "challenge-"+strconv.Itoa(fixtures.SecondFactorSamID), res.Challenge)

	// The earlier failure still counts: one more reaches the threshold of two.
	require.Equal(t, login.InvalidCredentials, r.password(t, fixtures.SecondFactorSamEmail, "wrong").Outcome)
	assert.Equal(t, login.Throttled, r.password(t, fixtures.SecondFactorSamEmail, "wrong").Outcome)
}

func TestASuccessWithoutASecondFactorResetsTheCounter(t *testing.T) {
	r := newRig(t)
	require.Equal(t, login.InvalidCredentials, r.password(t, fixtures.OwnerJohnEmail, "wrong").Outcome)
	require.Equal(t, login.Authenticated, r.password(t, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword).Outcome)
	require.Equal(t, login.InvalidCredentials, r.password(t, fixtures.OwnerJohnEmail, "wrong").Outcome)
	assert.Equal(t, login.InvalidCredentials, r.password(t, fixtures.OwnerJohnEmail, "wrong").Outcome, "the count restarted at the success")
}

func TestTheVerificationGatePrecedesTheChallengeAndCountsNothing(t *testing.T) {
	gate := func(c *login.Config[*ent.User]) {
		c.Gate = func(u *ent.User) bool { return u.Email == fixtures.SecondFactorSamEmail }
	}
	r := newRig(t, gate)
	// Whatever the password, a gated account gets neither a challenge nor a failure.
	for range 3 {
		assert.Equal(t, login.Gated, r.password(t, fixtures.SecondFactorSamEmail, fixtures.SecondFactorSamPassword).Outcome)
		assert.Equal(t, login.Gated, r.password(t, fixtures.SecondFactorSamEmail, "wrong").Outcome)
	}
	assert.Zero(t, delayOf(t, r.login, fixtures.SecondFactorSamEmail))
}

func TestAnErrorFromRecordingAFailureRefusesTheAttempt(t *testing.T) {
	r := newRig(t, func(c *login.Config[*ent.User]) { c.Throttle = failingCounter{c.Throttle} })
	res, err := r.module.Password(t.Context(), fixtures.OwnerJohnEmail, "wrong")
	require.ErrorIs(t, err, errCounter)
	assert.NotEqual(t, login.Authenticated, res.Outcome)
}

func TestUserAndStaffCountersAreSeparate(t *testing.T) {
	r := newRig(t)
	for range 2 {
		require.NoError(t, r.staff.RecordFailure(t.Context(), fixtures.OwnerJohnEmail))
	}
	assert.Equal(t, login.Authenticated, r.password(t, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword).Outcome)
}

var samChallenge = "challenge-" + strconv.Itoa(fixtures.SecondFactorSamID)

func TestABadChallengeIsRefusedWithoutCountingAFailure(t *testing.T) {
	r := newRig(t)
	for range 3 {
		assert.Equal(t, login.SecondInvalidChallenge, r.second(t, "forged", "123456").Outcome)
	}
	assert.Zero(t, delayOf(t, r.login, fixtures.SecondFactorSamEmail))
	assert.Zero(t, r.factor.verifyHit, "a bad challenge never reaches the code check")
}

func TestAChallengeThatMovedIsRefusedWithoutCountingAFailure(t *testing.T) {
	r := newRig(t)
	r.factor.moved = true
	for range 3 {
		assert.Equal(t, login.SecondInvalidChallenge, r.second(t, samChallenge, "123456").Outcome)
	}
	assert.Zero(t, delayOf(t, r.login, fixtures.SecondFactorSamEmail))
}

func TestAWrongCodeCountsAgainstTheBudgetOfAWrongPassword(t *testing.T) {
	r := newRig(t)
	require.Equal(t, login.InvalidCredentials, r.password(t, fixtures.SecondFactorSamEmail, "wrong").Outcome)
	assert.Equal(t, login.SecondInvalidCode, r.second(t, samChallenge, "000000").Outcome)
	// One password failure and one code failure reach the threshold of two.
	assert.Equal(t, login.SecondThrottled, r.second(t, samChallenge, "123456").Outcome, "a correct code does not skip the delay")
	assert.Equal(t, login.Throttled, r.password(t, fixtures.SecondFactorSamEmail, fixtures.SecondFactorSamPassword).Outcome)
}

func TestARecoveryCodeIsSpentOnce(t *testing.T) {
	r := newRig(t)
	assert.Equal(t, login.SecondAuthenticated, r.second(t, samChallenge, "recovery").Outcome)
	assert.Equal(t, login.SecondInvalidCode, r.second(t, samChallenge, "recovery").Outcome)
}

func TestAGoodCodeAuthenticatesAndResetsTheCounter(t *testing.T) {
	r := newRig(t)
	require.Equal(t, login.SecondInvalidCode, r.second(t, samChallenge, "000000").Outcome)
	res := r.second(t, samChallenge, "123456")
	require.Equal(t, login.SecondAuthenticated, res.Outcome)
	assert.Equal(t, int64(fixtures.SecondFactorSamID), res.Principal.ID)
	assert.Equal(t, "password", res.Method)

	require.Equal(t, login.SecondInvalidCode, r.second(t, samChallenge, "000000").Outcome)
	assert.Equal(t, login.SecondInvalidCode, r.second(t, samChallenge, "000000").Outcome, "the count restarted at the success")
}

func TestAFailingCounterResetAfterACommittedSecondFactorStillAuthenticates(t *testing.T) {
	r := newRig(t, func(c *login.Config[*ent.User]) { c.Throttle = resetFailing{c.Throttle} })
	assert.Equal(t, login.SecondAuthenticated, r.second(t, samChallenge, "123456").Outcome)
}

func TestAnErrorFromRecordingACodeFailureRefusesTheAttempt(t *testing.T) {
	r := newRig(t, func(c *login.Config[*ent.User]) { c.Throttle = failingCounter{c.Throttle} })
	res, err := r.module.SecondFactor(t.Context(), samChallenge, "000000")
	require.ErrorIs(t, err, errCounter)
	assert.NotEqual(t, login.SecondAuthenticated, res.Outcome)
}

func TestTheModuleImportsNoDatabaseClient(t *testing.T) {
	pkg, err := build.ImportDir(".", 0) // the package's own imports, tests excluded
	require.NoError(t, err)
	for _, path := range pkg.Imports {
		assert.False(t, strings.HasSuffix(path, "/sphericon/ent") || strings.Contains(path, "/ent/"), "imports %s", path)
		assert.NotEqual(t, "database/sql", path)
	}
}

func delayOf(t *testing.T, th login.Throttle, address string) time.Duration {
	t.Helper()
	d, err := th.Delay(t.Context(), address)
	require.NoError(t, err)
	return d
}
