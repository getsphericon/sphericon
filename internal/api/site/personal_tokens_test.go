package site_test

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	ht "github.com/ogen-go/ogen/http"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/apitoken"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Personal token (ADR 0061) is a Connection without the application: a member mints it on /site
// for their own scripts, and it calls /api as them, bounded by their current Role.

// mintPersonal mints, as the User email, a Personal token named name with scopes in the Workspace
// workspaceID, and returns the answer.
func mintPersonal(t *testing.T, env *testhelper.TestEnv, email string, workspaceID siteapi.WorkspaceId, name string, scopes ...siteapi.PersonalScope) siteapi.SitePersonalTokensCreateRes {
	t.Helper()
	res, err := env.SiteActor(t, email).SitePersonalTokensCreate(t.Context(),
		&siteapi.SiteCreatePersonalTokenInput{Name: name, Scopes: scopes},
		siteapi.SitePersonalTokensCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	return res
}

// mintedPersonal is mintPersonal that must succeed, and returns the token and its resource.
func mintedPersonal(t *testing.T, env *testhelper.TestEnv, email string, workspaceID siteapi.WorkspaceId, scopes ...siteapi.PersonalScope) *siteapi.SiteCreatePersonalTokenResponse {
	t.Helper()
	res := mintPersonal(t, env, email, workspaceID, "import script", scopes...)
	minted, ok := res.(*siteapi.SiteCreatePersonalTokenResponse)
	require.Truef(t, ok, "got %#v", res)
	return minted
}

// personalTokensOf is what the User email sees in the Personal tokens of Acme.
func personalTokensOf(t *testing.T, env *testhelper.TestEnv, email string) []siteapi.SitePersonalTokenResource {
	t.Helper()
	res, err := env.SiteActor(t, email).SitePersonalTokensList(t.Context(), siteapi.SitePersonalTokensListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	listed, ok := res.(*siteapi.SitePersonalTokensListOKApplicationJSON)
	require.Truef(t, ok, "got %#v", res)
	return *listed
}

// listContactsWith is the answer of /api to a contacts list on the bearer.
func listContactsWith(t *testing.T, env *testhelper.TestEnv, bearer string) externalapi.ContactsListRes {
	t.Helper()
	res, err := env.ExternalWithToken(t, bearer).ContactsList(t.Context(), externalapi.ContactsListParams{})
	require.NoError(t, err)
	return res
}

// demoteAdminAlice makes the Acme admin Alice a member.
func demoteAdminAlice(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	demoted, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteMembershipsUpdate(t.Context(),
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleMember},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.AcmeAdminMembershipID)})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteMembershipResource{}, demoted)
}

// A member mints a Personal token of her own: its secret is shown once, it calls /api within its
// scopes and it is hers, listed among her Personal tokens and neither with the Workspace's API
// tokens nor with the Connections.
func TestAMemberMintsAPersonalTokenOfHerOwn(t *testing.T) {
	env := testhelper.Setup(t)

	minted := mintedPersonal(t, env, fixtures.MemberMaryEmail, fixtures.AcmePublicID, siteapi.PersonalScopeContactsRead)

	assert.Equal(t, "import script", minted.Resource.Name)
	assert.Equal(t, []siteapi.PersonalScope{siteapi.PersonalScopeContactsRead}, minted.Resource.Scopes)
	assert.Equal(t, siteapi.EntityId(fixtures.MemberMaryID), minted.Resource.UserId)
	assert.Equal(t, fixtures.MemberMaryName, minted.Resource.UserName)
	listed := listContactsWith(t, env, minted.Token)
	assert.IsType(t, &externalapi.ContactsListOK{}, listed)
	row, err := env.DB.ApiToken.Query().Where(apitoken.Prefix(minted.Resource.Prefix)).Only(t.Context())
	require.NoError(t, err)
	assert.NotContains(t, row.SecretHash, minted.Token, "only a hash of the secret is stored")
	marysMembership := new(int64(fixtures.AcmeMemberMembershipID))
	assert.Equal(t, marysMembership, row.MembershipID)

	marys := personalTokensOf(t, env, fixtures.MemberMaryEmail)
	require.Len(t, marys, 1)
	assert.Equal(t, minted.Resource.ID, marys[0].ID)

	john := env.SiteActor(t, fixtures.OwnerJohnEmail)
	tokens, err := john.SiteTokensList(t.Context(), siteapi.SiteTokensListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	workspaceTokens, isList := tokens.(*siteapi.SiteTokensListOKApplicationJSON)
	require.Truef(t, isList, "got %#v", tokens)
	tokenIDs := lo.Map(*workspaceTokens, func(r siteapi.SiteApiTokenResource, _ int) siteapi.EntityId { return r.ID })
	assert.NotContains(t, tokenIDs, minted.Resource.ID, "a Personal token is not the Workspace's API token")
	connections, err := john.SiteConnectionsList(t.Context(), siteapi.SiteConnectionsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	connected, isList := connections.(*siteapi.SiteConnectionsListOKApplicationJSON)
	require.Truef(t, isList, "got %#v", connections)
	assert.Empty(t, *connected, "a Personal token is not a Connection")
}

// A Personal token can do no more than its scopes: a call that needs another one is refused.
func TestAPersonalTokenIsBoundedByItsScopes(t *testing.T) {
	env := testhelper.Setup(t)
	minted := mintedPersonal(t, env, fixtures.MemberMaryEmail, fixtures.AcmePublicID, siteapi.PersonalScopeContactsRead)

	res, err := env.ExternalWithToken(t, minted.Token).SegmentsList(t.Context(), externalapi.SegmentsListParams{})
	require.NoError(t, err)

	assert.IsType(t, &externalapi.SegmentsListForbidden{}, res)
}

// The mint form offers the scopes the caller's Role may grant: a member none whose Permission she
// lacks, an admin every one.
func TestTheGrantableScopesFollowTheRole(t *testing.T) {
	env := testhelper.Setup(t)
	params := siteapi.SitePersonalTokensGrantableScopesParams{WorkspaceId: fixtures.AcmePublicID}

	member, err := env.SiteActor(t, fixtures.MemberMaryEmail).SitePersonalTokensGrantableScopes(t.Context(), params)
	require.NoError(t, err)
	admin, err := env.SiteActor(t, fixtures.AdminAliceEmail).SitePersonalTokensGrantableScopes(t.Context(), params)
	require.NoError(t, err)

	memberListed, isList := member.(*siteapi.SitePersonalTokensGrantableScopesOKApplicationJSON)
	require.Truef(t, isList, "got %#v", member)
	adminListed, isList := admin.(*siteapi.SitePersonalTokensGrantableScopesOKApplicationJSON)
	require.Truef(t, isList, "got %#v", admin)
	memberScopes := []siteapi.PersonalScope(*memberListed)
	adminScopes := []siteapi.PersonalScope(*adminListed)
	everyPersonalScope := siteapi.PersonalScope("").AllValues()

	assert.Contains(t, memberScopes, siteapi.PersonalScopeContactsWrite)
	assert.NotContains(t, memberScopes, siteapi.PersonalScopeWebhooksRead)
	assert.NotContains(t, memberScopes, siteapi.PersonalScopeContactsExport)
	assert.NotContains(t, memberScopes, siteapi.PersonalScopeSendingDomainsWrite)
	assert.Equal(t, everyPersonalScope, adminScopes)
}

// A member cannot mint a scope whose Permission her Role lacks: the whole token is refused with
// 403 permission_required and nothing is stored.
func TestAPersonalTokenCarriesOnlyWhatTheRoleMayGrant(t *testing.T) {
	env := testhelper.Setup(t)

	res := mintPersonal(t, env, fixtures.MemberMaryEmail, fixtures.AcmePublicID, "hooks",
		siteapi.PersonalScopeContactsRead, siteapi.PersonalScopeWebhooksRead)

	refused, isRefused := res.(*siteapi.SitePersonalTokensCreateForbidden)
	require.Truef(t, isRefused, "got %#v", res)
	assert.Equal(t, siteapi.ProblemCodePermissionRequired, refused.Code.Value)
	stored, err := env.DB.ApiToken.Query().Where(apitoken.MembershipID(fixtures.AcmeMemberMembershipID)).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, stored)
}

// A Personal token never carries a scope only the Workspace's API token may hold, whatever the
// Role: tokens:write would let it mint a token that outlives its User, and contacts:erase and
// audit:read are no User's to give (ADR 0061). The contract types the scopes as PersonalScope, so
// even an owner's request naming one is not decoded (400), and nothing is stored.
func TestAPersonalTokenNeverCarriesATokenOnlyScope(t *testing.T) {
	for _, scope := range []externalapi.ApiTokenScope{externalapi.ApiTokenScopeTokensWrite, externalapi.ApiTokenScopeContactsErase, externalapi.ApiTokenScopeAuditRead} {
		t.Run(string(scope), func(t *testing.T) {
			env := testhelper.Setup(t)
			body := `{"name":"script","scopes":["contacts:read","` + string(scope) + `"]}`
			owner := env.SiteActorVia(t, fixtures.OwnerJohnEmail, func(inner ht.Client) ht.Client {
				return rawBody{inner: inner, body: body}
			})
			valid := &siteapi.SiteCreatePersonalTokenInput{Name: "script", Scopes: []siteapi.PersonalScope{siteapi.PersonalScopeContactsRead}}

			res, err := owner.SitePersonalTokensCreate(t.Context(), valid, siteapi.SitePersonalTokensCreateParams{WorkspaceId: fixtures.AcmePublicID})
			require.NoError(t, err)
			stored, err := env.DB.ApiToken.Query().Where(apitoken.MembershipID(fixtures.AcmeOwnerMembershipID)).Exist(t.Context())
			require.NoError(t, err)

			refused, isRefused := res.(*siteapi.SitePersonalTokensCreateBadRequest)
			require.Truef(t, isRefused, "got %#v", res)
			assert.EqualValues(t, http.StatusBadRequest, refused.Status.Value)
			assert.False(t, stored, "a refused mint stores nothing")
		})
	}
}

// The Role bounds a Personal token on every request: an admin's token to the webhook endpoints
// loses them on the request after the admin is made a member.
func TestADemotionTakesEffectOnAPersonalTokensNextRequest(t *testing.T) {
	env := testhelper.Setup(t)
	minted := mintedPersonal(t, env, fixtures.AdminAliceEmail, fixtures.AcmePublicID, siteapi.PersonalScopeWebhooksRead)
	api := env.ExternalWithToken(t, minted.Token)
	before, err := api.WebhooksList(t.Context(), externalapi.WebhooksListParams{})
	require.NoError(t, err)
	require.IsType(t, &externalapi.WebhooksListOK{}, before)

	demoteAdminAlice(t, env)
	after, err := api.WebhooksList(t.Context(), externalapi.WebhooksListParams{})
	require.NoError(t, err)

	refused, isRefused := after.(*externalapi.WebhooksListForbidden)
	require.Truef(t, isRefused, "got %#v", after)
	assert.Equal(t, externalapi.ProblemCodePermissionRequired, refused.Code.Value)
}

// assertPersonalTokenEnded checks that the Personal token is refused on /api and its row is gone.
func assertPersonalTokenEnded(t *testing.T, env *testhelper.TestEnv, minted *siteapi.SiteCreatePersonalTokenResponse) {
	t.Helper()
	res := listContactsWith(t, env, minted.Token)
	assert.IsType(t, &externalapi.ContactsListUnauthorized{}, res)
	left, err := env.DB.ApiToken.Query().Where(apitoken.Prefix(minted.Resource.Prefix)).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, left)
}

// A Personal token ends with its Membership: its User removed, leaving, or deleting themself.
func TestAPersonalTokenEndsWithItsMembership(t *testing.T) {
	for name, end := range map[string]func(t *testing.T, env *testhelper.TestEnv){
		"removal": func(t *testing.T, env *testhelper.TestEnv) {
			t.Helper()
			removed, err := env.SiteActor(t, fixtures.AdminAliceEmail).SiteMembershipsDelete(t.Context(),
				siteapi.SiteMembershipsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.AcmeMemberMembershipID)})
			require.NoError(t, err)
			require.IsType(t, &siteapi.SiteMembershipsDeleteNoContent{}, removed)
		},
		"leaving": func(t *testing.T, env *testhelper.TestEnv) {
			t.Helper()
			left, err := env.SiteActor(t, fixtures.MemberMaryEmail).SiteWorkspacesLeave(t.Context(), siteapi.SiteWorkspacesLeaveParams{WorkspaceId: fixtures.AcmePublicID})
			require.NoError(t, err)
			require.IsType(t, &siteapi.SiteWorkspacesLeaveNoContent{}, left)
		},
		"user deletion": func(t *testing.T, env *testhelper.TestEnv) {
			t.Helper()
			deleted, err := env.SiteActor(t, fixtures.MemberMaryEmail).SiteUserDeleteUser(t.Context(), &siteapi.SiteDeleteUserInput{
				Email:           fixtures.MemberMaryEmail,
				CurrentPassword: siteapi.NewOptString(fixtures.MemberMaryPassword),
			})
			require.NoError(t, err)
			require.IsType(t, &siteapi.SiteUserDeleteUserNoContent{}, deleted)
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			minted := mintedPersonal(t, env, fixtures.MemberMaryEmail, fixtures.AcmePublicID, siteapi.PersonalScopeContactsRead)
			before := listContactsWith(t, env, minted.Token)
			require.IsType(t, &externalapi.ContactsListOK{}, before)

			end(t, env)

			assertPersonalTokenEnded(t, env, minted)
		})
	}
}

// What a Personal token does is recorded in the Audit log as done by its User, named through the
// token (ADR 0061).
func TestAPersonalTokenActsAsItsUser(t *testing.T) {
	env := testhelper.Setup(t)
	minted := mintedPersonal(t, env, fixtures.MemberMaryEmail, fixtures.AcmePublicID, siteapi.PersonalScopeContactsWrite)

	_, err := env.ExternalWithToken(t, minted.Token).ContactsUpdate(t.Context(),
		&externalapi.UpdateContactInput{FirstName: externalapi.NewOptNilString("Renamed")},
		externalapi.ContactsUpdateParams{ID: externalapi.EntityId(fixtures.ContactBobID)})
	require.NoError(t, err)

	entries := lo.FilterMap(env.OutboxEvents(t, events.NameAuditEntry), func(e events.DomainEvent, _ int) (*events.AuditEntry, bool) {
		entry, ok := e.(*events.AuditEntry)
		return entry, ok && entry.Action == "contact.update"
	})
	mary := events.Actor{Kind: events.ActorUser, ID: strconv.Itoa(fixtures.MemberMaryID), Name: fixtures.MemberMaryName + " via import script"}
	require.Len(t, entries, 1)
	assert.Equal(t, mary, entries[0].Actor)
}

// Each User sees and revokes their own Personal tokens; members:manage sees and revokes anyone's.
func TestPersonalTokensAreTheirUsersOrAManagersToRevoke(t *testing.T) {
	env := testhelper.Setup(t)
	marys := mintedPersonal(t, env, fixtures.MemberMaryEmail, fixtures.AcmePublicID, siteapi.PersonalScopeContactsRead)
	alices := mintedPersonal(t, env, fixtures.AdminAliceEmail, fixtures.AcmePublicID, siteapi.PersonalScopeContactsRead)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)
	admin := env.SiteActor(t, fixtures.AdminAliceEmail)

	tokenIDs := func(r siteapi.SitePersonalTokenResource, _ int) siteapi.EntityId { return r.ID }

	marysView := personalTokensOf(t, env, fixtures.MemberMaryEmail)
	maryIDs := lo.Map(marysView, tokenIDs)
	assert.Equal(t, []siteapi.EntityId{marys.Resource.ID}, maryIDs, "a member sees only her own")
	refused, err := mary.SitePersonalTokensDelete(t.Context(), siteapi.SitePersonalTokensDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: alices.Resource.ID})
	require.NoError(t, err)
	notFound, isProblem := refused.(*siteapi.ProblemDetails)
	require.Truef(t, isProblem, "got %#v", refused)
	assert.EqualValues(t, http.StatusNotFound, notFound.Status.Value, "another User's Personal token is not the member's to find")
	alicesStillWorks := listContactsWith(t, env, alices.Token)
	assert.IsType(t, &externalapi.ContactsListOK{}, alicesStillWorks)

	adminsView := personalTokensOf(t, env, fixtures.AdminAliceEmail)
	adminIDs := lo.Map(adminsView, tokenIDs)
	assert.ElementsMatch(t, []siteapi.EntityId{marys.Resource.ID, alices.Resource.ID}, adminIDs, "members:manage sees anyone's")
	revoked, err := admin.SitePersonalTokensDelete(t.Context(), siteapi.SitePersonalTokensDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: marys.Resource.ID})
	require.NoError(t, err)
	marysAfter := listContactsWith(t, env, marys.Token)
	marysViewAfter := personalTokensOf(t, env, fixtures.MemberMaryEmail)

	assert.IsType(t, &siteapi.SitePersonalTokensDeleteNoContent{}, revoked)
	assert.IsType(t, &externalapi.ContactsListUnauthorized{}, marysAfter)
	assert.Empty(t, marysViewAfter, "a revoked Personal token is no longer listed")
}

// A Personal token of a User with a required acceptance pending is refused like a Connection
// (ADR 0062): MemberMary accepted only a stale version of the fixture terms.
func TestAPersonalTokenOfAGatedUserIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	minted := mintedPersonal(t, env, fixtures.MemberMaryEmail, fixtures.AcmePublicID, siteapi.PersonalScopeContactsRead)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	res := listContactsWith(t, env, minted.Token)

	refused, isRefused := res.(*externalapi.ContactsListForbidden)
	require.Truef(t, isRefused, "got %#v", res)
	assert.Equal(t, externalapi.ProblemCodeAcceptancePending, refused.Code.Value)
}

// The Workspace's Two-factor requirement applies to a Personal token as to its User's session:
// once the User's grace has ended without a Second factor, the token is refused (ADR 0020).
func TestTheTwoFactorRequirementWithholdsAPersonalToken(t *testing.T) {
	// Umbrella requires a Second factor since 2026-03-01; Rita's grace ends a week later.
	graceEnds := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	at := graceEnds.Add(-time.Hour)
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
	minted := mintedPersonal(t, env, fixtures.UmbrellaOwnerRitaEmail, fixtures.UmbrellaPublicID, siteapi.PersonalScopeContactsRead)
	inGrace := listContactsWith(t, env, minted.Token)
	require.IsType(t, &externalapi.ContactsListOK{}, inGrace)

	at = graceEnds.Add(time.Hour)
	res := listContactsWith(t, env, minted.Token)

	refused, isRefused := res.(*externalapi.ContactsListForbidden)
	require.Truef(t, isRefused, "got %#v", res)
	assert.Equal(t, externalapi.ProblemCodeSecondFactorRequired, refused.Code.Value)
}
