package site_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// inviteToAcme has Acme's Owner invite the address and returns the accept token.
func inviteToAcme(t *testing.T, env *testhelper.TestEnv, email string) string {
	t.Helper()
	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteInvitationsCreate(t.Context(),
		&siteapi.SiteCreateInvitationInput{Email: siteapi.EmailAddress(email), Role: siteapi.SiteInvitableRoleMember},
		siteapi.SiteInvitationsCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	created, ok := res.(*siteapi.SiteCreateInvitationResponse)
	require.Truef(t, ok, "got %T", res)
	return created.InviteUrl[strings.LastIndex(created.InviteUrl, "/")+1:]
}

func TestAnInvitationToAnUnverifiedAddressIsLikeOneToANewAddress(t *testing.T) {
	env := testhelper.Setup(t)
	register(t, env, "Squatter", "victim@example.com")
	token := inviteToAcme(t, env, "victim@example.com")

	lookup, err := env.SiteAnonymous(t).SitePublicInvitationsLookup(t.Context(),
		siteapi.SitePublicInvitationsLookupParams{Token: token})
	require.NoError(t, err)
	found, ok := lookup.(*siteapi.SiteInvitationLookupResult)
	require.Truef(t, ok, "got %T", lookup)
	assert.False(t, found.HasAccount, "an Unverified User is not an account")

	res, err := env.SiteAnonymous(t).SitePublicInvitationsAccept(t.Context(),
		&siteapi.SiteAcceptInvitationInput{}, siteapi.SitePublicInvitationsAcceptParams{Token: token})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SitePublicInvitationsAcceptUnprocessableEntity{}, res)

	res, err = env.SiteAnonymous(t).SitePublicInvitationsAccept(t.Context(),
		&siteapi.SiteAcceptInvitationInput{Name: siteapi.NewOptString("Victim")},
		siteapi.SitePublicInvitationsAcceptParams{Token: token})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SitePublicInvitationsAcceptUnprocessableEntity{}, res, "a password is required too")
}

// The reported pre-hijacking: the attacker registers the invitee's address, an Owner
// invites it, the invitee accepts. The attacker ends with no way in.
func TestPreHijackingAnAttackerWhoRegisteredTheInviteesAddressGainsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	register(t, env, "Attacker", "victim@example.com")
	token := inviteToAcme(t, env, "victim@example.com")

	res, err := env.SiteAnonymous(t).SitePublicInvitationsAccept(t.Context(),
		&siteapi.SiteAcceptInvitationInput{Name: siteapi.NewOptString("Victim"), Password: siteapi.NewOptString("victim-password-1")},
		siteapi.SitePublicInvitationsAcceptParams{Token: token})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SitePublicInvitationsAcceptOK{}, res)

	// Whatever the attacker tries as a password, there is none of theirs.
	attacker := postLogin(t, env, "victim@example.com", "attacker-password-1")
	assert.Equal(t, http.StatusUnauthorized, attacker.Code)
	assert.Nil(t, sessionCookie(attacker.Result()))

	// The invitee signs in with the password they set and holds the Membership.
	rec := postLogin(t, env, "victim@example.com", "victim-password-1")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	cookie := sessionCookie(rec.Result())
	require.NotNil(t, cookie)
	spaces, err := env.SiteWithToken(t, cookie.Value).SiteWorkspacesList(t.Context())
	require.NoError(t, err)
	slugs := make([]string, len(spaces))
	for i, s := range spaces {
		slugs[i] = s.Slug
	}
	assert.Contains(t, slugs, fixtures.AcmeSlug)
}

// An Invitation that proves an address is a Verified email like a registration link
// does, so the Operator reading the Audit log sees who proved what (ADR 0030).
func TestAcceptingAnInvitationThatVerifiesTheAddressIsAuditedAsEmailVerified(t *testing.T) {
	for name, prepare := range map[string]func(*testing.T, *testhelper.TestEnv){
		"new address":        func(*testing.T, *testhelper.TestEnv) {},
		"unverified address": func(t *testing.T, env *testhelper.TestEnv) { register(t, env, "Squatter", "invitee@example.com") },
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			prepare(t, env)
			token := inviteToAcme(t, env, "invitee@example.com")

			res, err := env.SiteAnonymous(t).SitePublicInvitationsAccept(t.Context(),
				&siteapi.SiteAcceptInvitationInput{Name: siteapi.NewOptString("Invitee"), Password: siteapi.NewOptString("invitee-password-1")},
				siteapi.SitePublicInvitationsAcceptParams{Token: token})
			require.NoError(t, err)
			require.IsType(t, &siteapi.SitePublicInvitationsAcceptOK{}, res)

			got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmeSlug, events.ActionUserEmailVerify)
			require.Len(t, got, 1)
			assert.Equal(t, "Invitee", got[0].Actor.Name.Value)
		})
	}
}
