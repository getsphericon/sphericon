package users_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/useridentity"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func usersModule(env *testhelper.TestEnv) *users.Module {
	return users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, time.Now)
}

// An address is one User's: users_email_key refuses a second User with it.
func TestCreatingAUserWithATakenAddressIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := usersModule(env).CreateUser(t.Context(), "Mallory", fixtures.OwnerJohnEmail, "hash")

	require.ErrorIs(t, err, users.ErrEmailTaken)
}

// An address taken between the change link and its confirmation is refused by users_email_key, and
// the User keeps the address they had.
func TestChangingTheEmailToATakenAddressIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := usersModule(env).ChangeEmail(t.Context(), fixtures.MemberMaryID, fixtures.OwnerJohnEmail)

	require.ErrorIs(t, err, users.ErrEmailTaken)
	mary, err := env.DB.User.Get(t.Context(), fixtures.MemberMaryID)
	require.NoError(t, err)
	assert.Equal(t, fixtures.MemberMaryEmail, mary.Email)
}

// A provider account linked to another User is refused by useridentity_provider_subject, and the
// User gains no identity.
func TestLinkingAProviderAccountOfAnotherUserIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	_, added, err := usersModule(env).LinkIdentity(t.Context(), fixtures.OwnerJohnID, fixtures.MaryGoogleIdentityProvider, fixtures.MaryGoogleIdentitySubject)

	require.ErrorIs(t, err, users.ErrIdentityInUse)
	assert.False(t, added)
	n, err := env.DB.UserIdentity.Query().Where(useridentity.UserID(fixtures.OwnerJohnID)).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, n)
}

// A provider account the User holds already changes nothing.
func TestLinkingAProviderAccountTheUserHoldsChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)

	_, added, err := usersModule(env).LinkIdentity(t.Context(), fixtures.MemberMaryID, fixtures.MaryGoogleIdentityProvider, fixtures.MaryGoogleIdentitySubject)

	require.NoError(t, err)
	assert.False(t, added)
}
