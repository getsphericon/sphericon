package workspacecreation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/workspacecreation"
)

// The verification link is single use even when two submissions race past the
// handler's own check: the write itself only lands on a still-Unverified User.
func TestVerifyEmailLandsOnlyOnAnUnverifiedUser(t *testing.T) {
	env := testhelper.Setup(t)
	acc := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords)
	creation := workspacecreation.New(env.Bus, acc, nil, testhelper.Passwords())
	u, err := acc.CreateUnverified(t.Context(), users.NewUser{Name: "Grace", Email: "grace@example.com", PasswordHash: "signed-up-hash"})
	require.NoError(t, err)

	_, _, err = creation.VerifyEmail(t.Context(), u.ID, new("hash-one"))
	require.NoError(t, err)
	_, _, err = creation.VerifyEmail(t.Context(), u.ID, new("hash-two"))
	require.ErrorIs(t, err, users.ErrAlreadyVerified)

	stored, err := acc.User(t.Context(), u.ID)
	require.NoError(t, err)
	assert.Equal(t, "hash-one", stored.PasswordHash, "the second submission overwrote nothing")
	assert.EqualValues(t, 1, stored.SessionEpoch, "verification ends earlier sessions once")
}
