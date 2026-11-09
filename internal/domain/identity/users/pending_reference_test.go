package users_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/authtoken"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func pendingLinks(t *testing.T, env *testhelper.TestEnv) *users.Links {
	t.Helper()
	acc := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, time.Now)
	return users.NewLinks(acc, authtoken.New("secret", time.Now))
}

// pending is the fixture pending sign-up with id as stored.
func pending(t *testing.T, env *testhelper.TestEnv, id int64) *ent.PendingSignup {
	t.Helper()
	p, err := env.DB.PendingSignup.Get(t.Context(), id)
	require.NoError(t, err)
	return p
}

// ADR 0062: the reference of a pending sign-up names it while it is open, and nothing once it is
// expired or gone; a forged one names nothing.
func TestAPendingSignupsReferenceNamesItWhileItIsOpen(t *testing.T) {
	env := testhelper.Setup(t)
	links := pendingLinks(t, env)
	open, err := links.PendingSignupReference(pending(t, env, fixtures.PendingSignupOpenID))
	require.NoError(t, err)
	expired, err := links.PendingSignupReference(pending(t, env, fixtures.PendingSignupExpiredID))
	require.NoError(t, err)

	got, err := links.PendingSignup(t.Context(), open)
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.PendingSignupOpenID, got.ID)
	_, err = links.PendingSignup(t.Context(), expired)
	require.ErrorIs(t, err, users.ErrPendingSignupGone)
	_, err = links.PendingSignup(t.Context(), "forged")
	require.ErrorIs(t, err, users.ErrPendingSignupGone)
}

// Declining by the reference deletes the pending sign-up; a reference that names nothing open any
// more is declined already.
func TestDecliningByTheReferenceDeletesThePendingSignupOnce(t *testing.T) {
	env := testhelper.Setup(t)
	links := pendingLinks(t, env)
	ref, err := links.PendingSignupReference(pending(t, env, fixtures.PendingSignupOpenID))
	require.NoError(t, err)

	require.NoError(t, links.DeclinePendingSignup(t.Context(), ref))

	assert.False(t, pendingExists(t, env, fixtures.PendingSignupOpenID))
	require.NoError(t, links.DeclinePendingSignup(t.Context(), ref), "declined already")
	require.NoError(t, links.DeclinePendingSignup(t.Context(), "forged"))
}

func pendingExists(t *testing.T, env *testhelper.TestEnv, id int64) bool {
	t.Helper()
	_, err := env.DB.PendingSignup.Get(t.Context(), id)
	if ent.IsNotFound(err) {
		return false
	}
	require.NoError(t, err)
	return true
}
