package site_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entinvitation "github.com/getsphericon/sphericon/ent/invitation"
	entuser "github.com/getsphericon/sphericon/ent/user"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An address is one account whatever case it is typed in (internal/emailcase): it is stored
// lower-cased, so a second Sign-up in another case finds the first one instead of making a
// second account that sign-in could never reach.
func TestAnAddressInAnotherCaseIsTheSameAccount(t *testing.T) {
	env := gateOff(t)

	assert.IsType(t, &siteapi.SiteAuthSignUpAccepted{}, signUpWithPassword(t, env, "Grace", "Grace@Example.com", "grace-password-1"))
	assert.IsType(t, &siteapi.SiteAuthSignUpConflict{}, signUpWithPassword(t, env, "Grace", "grace@EXAMPLE.com", "grace-password-2"))

	users, err := env.DB.User.Query().Where(entuser.EmailEqualFold("grace@example.com")).All(t.Context())
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, "grace@example.com", users[0].Email)
	assert.Equal(t, http.StatusOK, loginStatus(t, env, "GRACE@example.com", "grace-password-1"))
}

func TestAnInvitationIsStoredLowerCased(t *testing.T) {
	env := testhelper.Setup(t)

	inviteToAcme(t, env, "NewUser@Example.com")

	exists, err := env.DB.Invitation.Query().Where(entinvitation.Email("newuser@example.com")).Exist(t.Context())
	require.NoError(t, err)
	assert.True(t, exists)

	again, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteInvitationsCreate(t.Context(),
		&siteapi.SiteCreateInvitationInput{Email: "newuser@example.com", Role: siteapi.SiteInvitableRoleMember},
		siteapi.SiteInvitationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteCreateInvitationResponse{}, again)
	count, err := env.DB.Invitation.Query().Where(entinvitation.EmailEqualFold("newuser@example.com")).Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, count, "the same address in another case renews the one invitation")
}
