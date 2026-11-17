package users_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/useridentity"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An address is one User's: users_email_key refuses a second User with it.
func TestCreatingAUserWithATakenAddressIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := testhelper.Module[*users.Module](env).CreateUser(t.Context(), users.CreateUserInput{Name: "Mallory", Email: fixtures.OwnerJohnEmail, PasswordHash: "hash"})

	require.ErrorIs(t, err, users.ErrEmailTaken)
}

// An address taken between the change link and its confirmation is refused by users_email_key, and
// the User keeps the address they had.
func TestChangingTheEmailToATakenAddressIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := testhelper.Module[*users.Module](env).ChangeEmail(t.Context(), fixtures.MemberMaryID, fixtures.OwnerJohnEmail)

	require.ErrorIs(t, err, users.ErrEmailTaken)
	mary, err := env.DB.User.Get(t.Context(), fixtures.MemberMaryID)
	require.NoError(t, err)
	assert.Equal(t, fixtures.MemberMaryEmail, mary.Email)
}

// A provider account linked to another User is refused by useridentity_provider_subject, and the
// User gains no identity.
func TestLinkingAProviderAccountOfAnotherUserIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	link, err := testhelper.Module[*users.Module](env).LinkIdentity(t.Context(), users.LinkIdentityInput{UserID: fixtures.OwnerJohnID, Provider: fixtures.MaryGoogleIdentityProvider, Subject: fixtures.MaryGoogleIdentitySubject})

	require.ErrorIs(t, err, users.ErrIdentityInUse)
	assert.False(t, link.Added)
	n, err := env.DB.UserIdentity.Query().Where(useridentity.UserID(fixtures.OwnerJohnID)).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, n)
}

// A provider account the User holds already changes nothing.
func TestLinkingAProviderAccountTheUserHoldsChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)

	link, err := testhelper.Module[*users.Module](env).LinkIdentity(t.Context(), users.LinkIdentityInput{UserID: fixtures.MemberMaryID, Provider: fixtures.MaryGoogleIdentityProvider, Subject: fixtures.MaryGoogleIdentitySubject})

	require.NoError(t, err)
	assert.False(t, link.Added)
}
