package login_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/login"
)

// passkeyAdapter is the adapter's Passkey side over the fixtures' Users: a response is
// "user-<id>" for a credential of that User, "forged-<id>" for one that does not verify, and
// anything else names no credential.
type passkeyAdapter struct {
	db         *ent.Client
	verifyHits int
}

func (p *passkeyAdapter) Find(ctx context.Context, a login.PasskeyAssertion) (login.Pending[*ent.User], error) {
	id, ok := strings.CutPrefix(string(a.Response), "user-")
	if !ok {
		id, ok = strings.CutPrefix(string(a.Response), "forged-")
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if !ok || err != nil {
		return login.Pending[*ent.User]{}, login.ErrPasskeyUnknown
	}
	u, err := p.db.User.Get(ctx, n)
	if err != nil {
		return login.Pending[*ent.User]{}, login.ErrPasskeyUnknown
	}
	return login.Pending[*ent.User]{Principal: u, Address: u.Email}, nil
}

func (p *passkeyAdapter) Verify(_ context.Context, _ login.Pending[*ent.User], a login.PasskeyAssertion) error {
	p.verifyHits++
	if strings.HasPrefix(string(a.Response), "forged-") {
		return login.ErrPasskeyInvalid
	}
	return nil
}

func newPasskeyRig(t *testing.T, opts ...func(*login.Config[*ent.User])) (rig, *passkeyAdapter) {
	t.Helper()
	adapter := &passkeyAdapter{}
	withPasskeys := func(c *login.Config[*ent.User]) {
		c.PasskeyFind, c.PasskeyVerify = adapter.Find, adapter.Verify
	}
	r := newRig(t, append([]func(*login.Config[*ent.User]){withPasskeys}, opts...)...)
	adapter.db = r.db
	return r, adapter
}

func (r rig) passkey(t *testing.T, response string) login.PasskeyResult[*ent.User] {
	t.Helper()
	res, err := r.module.Passkey(t.Context(), login.PasskeyAssertion{Response: []byte(response)})
	require.NoError(t, err)
	return res
}

// passkeyErr is the refusal of a Passkey login that does not authenticate.
func (r rig) passkeyErr(t *testing.T, response string) error {
	t.Helper()
	_, err := r.module.Passkey(t.Context(), login.PasskeyAssertion{Response: []byte(response)})
	require.Error(t, err)
	return err
}

func userResponse(id int) string { return "user-" + strconv.Itoa(id) }

func forgedResponse(id int) string { return "forged-" + strconv.Itoa(id) }

func TestAVerifiedPasskeyAuthenticatesWithoutASecondStep(t *testing.T) {
	r, _ := newPasskeyRig(t)

	res := r.passkey(t, userResponse(fixtures.SecondFactorSamID))

	assert.Equal(t, login.PasskeyAuthenticated, res.Outcome, "a Second factor does not follow a Passkey")
	assert.Equal(t, int64(fixtures.SecondFactorSamID), res.Principal.ID)
}

func TestAResponseThatNamesNoCredentialFailsAndMovesNoCounter(t *testing.T) {
	r, adapter := newPasskeyRig(t)

	err := r.passkeyErr(t, "no credential")

	require.ErrorIs(t, err, login.ErrPasskeyFailed)
	assert.Zero(t, adapter.verifyHits, "nothing is verified for an unknown credential")
	assert.Zero(t, delayOf(t, r.login, fixtures.OwnerJohnEmail))
}

func TestAResponseThatDoesNotVerifyFailsAndCountsNothing(t *testing.T) {
	r, _ := newPasskeyRig(t)
	for range 3 {
		require.ErrorIs(t, r.passkeyErr(t, forgedResponse(fixtures.OwnerJohnID)), login.ErrPasskeyFailed)
	}

	assert.Zero(t, delayOf(t, r.login, fixtures.OwnerJohnEmail))
	assert.Equal(t, login.PasskeyAuthenticated, r.passkey(t, userResponse(fixtures.OwnerJohnID)).Outcome)
}

func TestTheThrottleOfTheUserFoundPrecedesTheVerification(t *testing.T) {
	r, adapter := newPasskeyRig(t)
	for range 2 {
		require.ErrorIs(t, r.passwordErr(t, fixtures.OwnerJohnEmail, "wrong"), login.ErrInvalidCredentials)
	}

	res := r.passkey(t, userResponse(fixtures.OwnerJohnID))

	assert.Equal(t, login.PasskeyThrottled, res.Outcome)
	assert.Positive(t, res.Wait)
	assert.Equal(t, 2, res.Limit)
	assert.Zero(t, adapter.verifyHits, "a correct response is not even checked while the delay runs")
}

func TestAPasskeyLoginWorksAgainOnceTheDelayHasPassed(t *testing.T) {
	r, _ := newPasskeyRig(t)
	for range 2 {
		require.ErrorIs(t, r.passwordErr(t, fixtures.OwnerJohnEmail, "wrong"), login.ErrInvalidCredentials)
	}
	require.Equal(t, login.PasskeyThrottled, r.passkey(t, userResponse(fixtures.OwnerJohnID)).Outcome)

	r.clock.t = r.clock.t.Add(2 * time.Second)

	assert.Equal(t, login.PasskeyAuthenticated, r.passkey(t, userResponse(fixtures.OwnerJohnID)).Outcome)
}

func TestTheVerificationGateRefusesAPasskeyLoginLikeAnyOtherFailure(t *testing.T) {
	gate := func(c *login.Config[*ent.User]) {
		c.Gate = func(u *ent.User) bool { return u.Email == fixtures.SecondFactorSamEmail }
	}
	r, adapter := newPasskeyRig(t, gate)

	err := r.passkeyErr(t, userResponse(fixtures.SecondFactorSamID))

	require.ErrorIs(t, err, login.ErrPasskeyFailed, "not the verification refusal: a Passkey login does not tell that the address waits")
	assert.Zero(t, adapter.verifyHits)
	assert.Zero(t, delayOf(t, r.login, fixtures.SecondFactorSamEmail))
}

func TestAVerifiedPasskeyResetsTheCounter(t *testing.T) {
	r, _ := newPasskeyRig(t)
	require.ErrorIs(t, r.passwordErr(t, fixtures.OwnerJohnEmail, "wrong"), login.ErrInvalidCredentials)
	require.Equal(t, login.PasskeyAuthenticated, r.passkey(t, userResponse(fixtures.OwnerJohnID)).Outcome)

	require.ErrorIs(t, r.passwordErr(t, fixtures.OwnerJohnEmail, "wrong"), login.ErrInvalidCredentials)
	require.ErrorIs(t, r.passwordErr(t, fixtures.OwnerJohnEmail, "wrong"), login.ErrInvalidCredentials, "the count restarted at the Passkey login")
}

func TestAFailedCounterResetDoesNotCostAVerifiedPasskeyLogin(t *testing.T) {
	r, _ := newPasskeyRig(t, func(c *login.Config[*ent.User]) { c.Throttle = resetFailing{c.Throttle} })

	res := r.passkey(t, userResponse(fixtures.OwnerJohnID))

	assert.Equal(t, login.PasskeyAuthenticated, res.Outcome)
}
