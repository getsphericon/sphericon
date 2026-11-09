package login_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/login"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/ratelimit"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// proveErr is the refusal of a proof that does not hold.
func (r rig) proveErr(t *testing.T, address, hash, password string) error {
	t.Helper()
	err := r.module.ProvePassword(t.Context(), address, hash, password)
	require.Error(t, err)
	return err
}

func TestProvingTheRightPasswordHolds(t *testing.T) {
	r := newRig(t)
	hash := hashOf(t, fixtures.OwnerJohnPassword)
	require.NoError(t, r.module.ProvePassword(t.Context(), fixtures.OwnerJohnEmail, hash, fixtures.OwnerJohnPassword))
}

func TestProofFailuresShareTheLoginBudget(t *testing.T) {
	r := newRig(t)
	hash := hashOf(t, fixtures.OwnerJohnPassword)
	require.ErrorIs(t, r.proveErr(t, fixtures.OwnerJohnEmail, hash, "wrong"), login.ErrPasswordIncorrect)
	require.ErrorIs(t, r.passwordErr(t, fixtures.OwnerJohnEmail, "wrong"), login.ErrInvalidCredentials)

	assert.Positive(t, waitOf(t, r.proveErr(t, fixtures.OwnerJohnEmail, hash, fixtures.OwnerJohnPassword)).Wait, "a right password is refused while the delay runs")
	waitOf(t, r.passwordErr(t, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword))
}

func TestProvingAnAccountWithoutAPasswordIsWrongAndCounted(t *testing.T) {
	r := newRig(t)
	const address = "passwordless@login.test"
	require.ErrorIs(t, r.proveErr(t, address, "", "anything"), login.ErrPasswordIncorrect)
	require.ErrorIs(t, r.proveErr(t, address, "", "anything"), login.ErrPasswordIncorrect)
	waitOf(t, r.proveErr(t, address, "", "anything"))
}

func hashOf(t *testing.T, password string) string {
	t.Helper()
	hash, err := testhelper.Passwords().Hash(password)
	require.NoError(t, err)
	return hash
}

func TestProvingAPasswordAnswersARunningDelayAsTheLoginAccountsRateLimit(t *testing.T) {
	r := newRig(t)
	hash := hashOf(t, fixtures.OwnerJohnPassword)
	require.NoError(t, r.module.ProvePassword(t.Context(), fixtures.OwnerJohnEmail, hash, fixtures.OwnerJohnPassword))
	require.ErrorIs(t, r.module.ProvePassword(t.Context(), fixtures.OwnerJohnEmail, hash, "wrong"), login.ErrPasswordIncorrect)
	require.ErrorIs(t, r.module.ProvePassword(t.Context(), fixtures.OwnerJohnEmail, hash, "wrong"), login.ErrPasswordIncorrect)

	err := r.module.ProvePassword(t.Context(), fixtures.OwnerJohnEmail, hash, fixtures.OwnerJohnPassword)

	wait, ok := errors.AsType[*ratelimit.WaitError](err)
	require.True(t, ok, "a right password is refused while the delay runs, got %v", err)
	assert.Equal(t, ratelimit.PolicyLoginAccount, wait.Policy)
	assert.Positive(t, wait.Wait)
	assert.Equal(t, 2, wait.Limit)
	assert.Equal(t, r.clock.now(), wait.At)
}
