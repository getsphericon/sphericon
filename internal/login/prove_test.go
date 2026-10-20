package login_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/login"
)

func (r rig) prove(t *testing.T, address, hash, password string) login.ProofResult {
	t.Helper()
	res, err := r.module.Prove(t.Context(), address, hash, password)
	require.NoError(t, err)
	return res
}

func TestProvingTheRightPasswordHolds(t *testing.T) {
	r := newRig(t)
	hash := hashOf(t, fixtures.OwnerJohnPassword)
	assert.Equal(t, login.ProofHolds, r.prove(t, fixtures.OwnerJohnEmail, hash, fixtures.OwnerJohnPassword).Proof)
}

func TestProofFailuresShareTheLoginBudget(t *testing.T) {
	r := newRig(t)
	hash := hashOf(t, fixtures.OwnerJohnPassword)
	require.Equal(t, login.ProofWrong, r.prove(t, fixtures.OwnerJohnEmail, hash, "wrong").Proof)
	require.Equal(t, login.InvalidCredentials, r.password(t, fixtures.OwnerJohnEmail, "wrong").Outcome)

	res := r.prove(t, fixtures.OwnerJohnEmail, hash, fixtures.OwnerJohnPassword)
	assert.Equal(t, login.ProofThrottled, res.Proof, "a right password is refused while the delay runs")
	assert.Positive(t, res.Wait)
	assert.Equal(t, login.Throttled, r.password(t, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword).Outcome)
}

func TestProvingAnAccountWithoutAPasswordIsWrongAndCounted(t *testing.T) {
	r := newRig(t)
	const address = "passwordless@login.test"
	assert.Equal(t, login.ProofWrong, r.prove(t, address, "", "anything").Proof)
	assert.Equal(t, login.ProofWrong, r.prove(t, address, "", "anything").Proof)
	assert.Equal(t, login.ProofThrottled, r.prove(t, address, "", "anything").Proof)
}

func hashOf(t *testing.T, password string) string {
	t.Helper()
	hash, err := credentials.HashPassword(password)
	require.NoError(t, err)
	return hash
}
