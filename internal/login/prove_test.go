package login_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/login"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func (r rig) prove(t *testing.T, address, hash, password string) login.ProofResult {
	t.Helper()
	res, err := r.module.Prove(t.Context(), address, hash, password)
	require.NoError(t, err)
	return res
}

// proveErr is the refusal of a proof that does not hold.
func (r rig) proveErr(t *testing.T, address, hash, password string) error {
	t.Helper()
	_, err := r.module.Prove(t.Context(), address, hash, password)
	require.Error(t, err)
	return err
}

func TestProvingTheRightPasswordHolds(t *testing.T) {
	r := newRig(t)
	hash := hashOf(t, fixtures.OwnerJohnPassword)
	assert.Equal(t, login.ProofHolds, r.prove(t, fixtures.OwnerJohnEmail, hash, fixtures.OwnerJohnPassword).Proof)
}

func TestProofFailuresShareTheLoginBudget(t *testing.T) {
	r := newRig(t)
	hash := hashOf(t, fixtures.OwnerJohnPassword)
	require.ErrorIs(t, r.proveErr(t, fixtures.OwnerJohnEmail, hash, "wrong"), login.ErrPasswordIncorrect)
	require.ErrorIs(t, r.passwordErr(t, fixtures.OwnerJohnEmail, "wrong"), login.ErrInvalidCredentials)

	res := r.prove(t, fixtures.OwnerJohnEmail, hash, fixtures.OwnerJohnPassword)
	assert.Equal(t, login.ProofThrottled, res.Proof, "a right password is refused while the delay runs")
	assert.Positive(t, res.Wait)
	assert.Equal(t, login.Throttled, r.password(t, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword).Outcome)
}

func TestProvingAnAccountWithoutAPasswordIsWrongAndCounted(t *testing.T) {
	r := newRig(t)
	const address = "passwordless@login.test"
	require.ErrorIs(t, r.proveErr(t, address, "", "anything"), login.ErrPasswordIncorrect)
	require.ErrorIs(t, r.proveErr(t, address, "", "anything"), login.ErrPasswordIncorrect)
	assert.Equal(t, login.ProofThrottled, r.prove(t, address, "", "anything").Proof)
}

func hashOf(t *testing.T, password string) string {
	t.Helper()
	hash, err := testhelper.Passwords().Hash(password)
	require.NoError(t, err)
	return hash
}
