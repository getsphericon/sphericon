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
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Personal token (ADR 0061) is a Connection without the application: a member mints it on /site
// for their own scripts, and it calls /api as them, bounded by their current Role.

// mintPersonal mints, as the User email, a Personal token named name with scopes in the Workspace
// workspaceID, and returns the answer.
func mintPersonal(t *testing.T, env *testhelper.TestEnv, email string, workspaceID siteapi.WorkspaceId, name string, scopes ...siteapi.PersonalScope) (*siteapi.SiteCreatePersonalTokenResponse, error) {
	t.Helper()
	return env.SiteActor(t, email).SitePersonalTokensCreate(t.Context(),
		&siteapi.SiteCreatePersonalTokenInput{Name: name, Scopes: scopes},
		siteapi.SitePersonalTokensCreateParams{WorkspaceId: workspaceID})
}

// mintedPersonal is mintPersonal that must succeed, and returns the token and its resource.
func mintedPersonal(t *testing.T, env *testhelper.TestEnv, email string, workspaceID siteapi.WorkspaceId, scopes ...siteapi.PersonalScope) *siteapi.SiteCreatePersonalTokenResponse {
	t.Helper()
	minted, err := mintPersonal(t, env, email, workspaceID, "import script", scopes...)
	require.NoError(t, err)
	return minted
}

// personalTokensOf is what the User email sees in the Personal tokens of the Workspace workspaceID.
func personalTokensOf(t *testing.T, env *testhelper.TestEnv, email string, workspaceID siteapi.WorkspaceId) []siteapi.SitePersonalTokenResource {
	t.Helper()
	listed, err := env.SiteActor(t, email).SitePersonalTokensList(t.Context(), siteapi.SitePersonalTokensListParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	return listed
}

// listContactsWith is the error /api answers a contacts list on the bearer with: nil when it lists.
func listContactsWith(t *testing.T, env *testhelper.TestEnv, bearer string) error {
	t.Helper()
	_, err := env.ExternalWithToken(t, bearer).ContactsList(t.Context(), externalapi.ContactsListParams{})
	return err
}

// The fixture Personal tokens (fixtures/api_tokens.yml), as their Users were shown them at minting.
var (
	melsToken  = credentials.TokenValue(fixtures.VehementMemberPersonalTokenPrefix, fixtures.VehementMemberPersonalTokenSecret)
	adasToken  = credentials.TokenValue(fixtures.VehementAdminPersonalTokenPrefix, fixtures.VehementAdminPersonalTokenSecret)
	ritasToken = credentials.TokenValue(fixtures.UmbrellaRitaPersonalTokenPrefix, fixtures.UmbrellaRitaPersonalTokenSecret)
)

// demoteVehementAdmin makes the Vehement admin Ada a member.
func demoteVehementAdmin(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	_, err := env.SiteActor(t, fixtures.VehementOwnerValEmail).SiteMembershipsUpdate(t.Context(),
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleMember},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.VehementPublicID, ID: siteapi.EntityId(fixtures.VehementAdminMembershipID)})
	require.NoError(t, err)
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
	require.NoError(t, listed)
	row, err := env.DB.ApiToken.Query().Where(apitoken.Prefix(minted.Resource.Prefix)).Only(t.Context())
	require.NoError(t, err)
	assert.NotContains(t, row.SecretHash, minted.Token, "only a hash of the secret is stored")
	marysMembership := new(int64(fixtures.AcmeMemberMembershipID))
	assert.Equal(t, marysMembership, row.MembershipID)

	marys := personalTokensOf(t, env, fixtures.MemberMaryEmail, fixtures.AcmePublicID)
	require.Len(t, marys, 1)
	assert.Equal(t, minted.Resource.ID, marys[0].ID)

	john := env.SiteActor(t, fixtures.OwnerJohnEmail)
	workspaceTokens, err := john.SiteTokensList(t.Context(), siteapi.SiteTokensListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	tokenIDs := lo.Map(workspaceTokens, func(r siteapi.SiteApiTokenResource, _ int) siteapi.EntityId { return r.ID })
	assert.NotContains(t, tokenIDs, minted.Resource.ID, "a Personal token is not the Workspace's API token")
	connected, err := john.SiteConnectionsList(t.Context(), siteapi.SiteConnectionsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.Empty(t, connected, "a Personal token is not a Connection")
}

// A Personal token can do no more than its scopes: a call that needs another one is refused.
func TestAPersonalTokenIsBoundedByItsScopes(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.ExternalWithToken(t, melsToken).SegmentsList(t.Context(), externalapi.SegmentsListParams{})

	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
}

// The mint form offers the scopes the caller's Role may grant: a member none whose Permission she
// lacks, an admin every one.
func TestTheGrantableScopesFollowTheRole(t *testing.T) {
	env := testhelper.Setup(t)
	params := siteapi.SitePersonalTokensGrantableScopesParams{WorkspaceId: fixtures.AcmePublicID}

	memberScopes, err := env.SiteActor(t, fixtures.MemberMaryEmail).SitePersonalTokensGrantableScopes(t.Context(), params)
	require.NoError(t, err)
	adminScopes, err := env.SiteActor(t, fixtures.AdminAliceEmail).SitePersonalTokensGrantableScopes(t.Context(), params)
	require.NoError(t, err)

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

	_, err := mintPersonal(t, env, fixtures.MemberMaryEmail, fixtures.AcmePublicID, "hooks",
		siteapi.PersonalScopeContactsRead, siteapi.PersonalScopeWebhooksRead)

	refused := testhelper.SiteRefusal(t, err, http.StatusForbidden)
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

			_, mintErr := owner.SitePersonalTokensCreate(t.Context(), valid, siteapi.SitePersonalTokensCreateParams{WorkspaceId: fixtures.AcmePublicID})
			stored, err := env.DB.ApiToken.Query().Where(apitoken.MembershipID(fixtures.AcmeOwnerMembershipID)).Exist(t.Context())
			require.NoError(t, err)

			refused := testhelper.SiteRefusal(t, mintErr, http.StatusBadRequest)
			assert.EqualValues(t, http.StatusBadRequest, refused.Status.Value)
			assert.False(t, stored, "a refused mint stores nothing")
		})
	}
}

// The Role bounds a Personal token on every request: an admin's token to the webhook endpoints
// loses them on the request after the admin is made a member.
func TestADemotionTakesEffectOnAPersonalTokensNextRequest(t *testing.T) {
	env := testhelper.Setup(t)
	api := env.ExternalWithToken(t, adasToken)
	_, err := api.WebhooksList(t.Context(), externalapi.WebhooksListParams{})
	require.NoError(t, err)

	demoteVehementAdmin(t, env)
	_, err = api.WebhooksList(t.Context(), externalapi.WebhooksListParams{})

	refused := testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	assert.Equal(t, externalapi.ProblemCodePermissionRequired, refused.Code.Value)
}

// assertPersonalTokenEnded checks that Mel's Personal token is refused on /api and its row is gone.
func assertPersonalTokenEnded(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	testhelper.ExternalRefusal(t, listContactsWith(t, env, melsToken), http.StatusUnauthorized)
	left, err := env.DB.ApiToken.Query().Where(apitoken.ID(fixtures.VehementMemberPersonalTokenID)).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, left)
}

// A Personal token ends with its Membership: its User removed, leaving, or deleting themself.
func TestAPersonalTokenEndsWithItsMembership(t *testing.T) {
	for name, end := range map[string]func(t *testing.T, env *testhelper.TestEnv){
		"removal": func(t *testing.T, env *testhelper.TestEnv) {
			t.Helper()
			err := env.SiteActor(t, fixtures.VehementAdminAdaEmail).SiteMembershipsDelete(t.Context(),
				siteapi.SiteMembershipsDeleteParams{WorkspaceId: fixtures.VehementPublicID, ID: siteapi.EntityId(fixtures.VehementMemberMembershipID)})
			require.NoError(t, err)
		},
		"leaving": func(t *testing.T, env *testhelper.TestEnv) {
			t.Helper()
			err := env.SiteActor(t, fixtures.VehementMemberMelEmail).SiteWorkspacesLeave(t.Context(), siteapi.SiteWorkspacesLeaveParams{WorkspaceId: fixtures.VehementPublicID})
			require.NoError(t, err)
		},
		"user deletion": func(t *testing.T, env *testhelper.TestEnv) {
			t.Helper()
			_, err := env.SiteActor(t, fixtures.VehementMemberMelEmail).SiteUserDeleteUser(t.Context(), &siteapi.SiteDeleteUserInput{
				Email:           fixtures.VehementMemberMelEmail,
				CurrentPassword: siteapi.NewOptString(fixtures.VehementMemberMelPassword),
			})
			require.NoError(t, err)
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			before := listContactsWith(t, env, melsToken)
			require.NoError(t, before)

			end(t, env)

			assertPersonalTokenEnded(t, env)
		})
	}
}

// What a Personal token does is recorded in the Audit log as done by its User, named through the
// token (ADR 0061).
func TestAPersonalTokenActsAsItsUser(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.ExternalWithToken(t, melsToken).ContactsUpdate(t.Context(),
		&externalapi.UpdateContactInput{FirstName: externalapi.NewOptNilString("Renamed")},
		externalapi.ContactsUpdateParams{ID: externalapi.EntityId(fixtures.ContactVehementVicID)})
	require.NoError(t, err)

	entries := lo.FilterMap(env.QueuedEvents(t, events.NameAuditEntry), func(e events.DomainEvent, _ int) (*events.AuditEntry, bool) {
		entry, ok := e.(*events.AuditEntry)
		return entry, ok && entry.Action == "contact.update"
	})
	mel := events.Actor{Kind: events.ActorUser, ID: strconv.Itoa(fixtures.VehementMemberMelID), Name: fixtures.VehementMemberMelName + " via " + fixtures.VehementMemberPersonalTokenName}
	require.Len(t, entries, 1)
	assert.Equal(t, mel, entries[0].Actor)
}

// Each User sees and revokes their own Personal tokens; members:manage sees and revokes anyone's.
func TestPersonalTokensAreTheirUsersOrAManagersToRevoke(t *testing.T) {
	env := testhelper.Setup(t)
	mel := env.SiteActor(t, fixtures.VehementMemberMelEmail)
	admin := env.SiteActor(t, fixtures.VehementAdminAdaEmail)
	mels := siteapi.EntityId(fixtures.VehementMemberPersonalTokenID)
	adas := siteapi.EntityId(fixtures.VehementAdminPersonalTokenID)

	tokenIDs := func(r siteapi.SitePersonalTokenResource, _ int) siteapi.EntityId { return r.ID }

	melsView := personalTokensOf(t, env, fixtures.VehementMemberMelEmail, fixtures.VehementPublicID)
	melIDs := lo.Map(melsView, tokenIDs)
	assert.Equal(t, []siteapi.EntityId{mels}, melIDs, "a member sees only her own")
	err := mel.SitePersonalTokensDelete(t.Context(), siteapi.SitePersonalTokensDeleteParams{WorkspaceId: fixtures.VehementPublicID, ID: adas})
	testhelper.SiteRefusal(t, err, http.StatusNotFound, "another User's Personal token is not the member's to find")
	adasStillWorks := listContactsWith(t, env, adasToken)
	require.NoError(t, adasStillWorks)

	adminsView := personalTokensOf(t, env, fixtures.VehementAdminAdaEmail, fixtures.VehementPublicID)
	adminIDs := lo.Map(adminsView, tokenIDs)
	assert.ElementsMatch(t, []siteapi.EntityId{mels, adas}, adminIDs, "members:manage sees anyone's")
	revoked := admin.SitePersonalTokensDelete(t.Context(), siteapi.SitePersonalTokensDeleteParams{WorkspaceId: fixtures.VehementPublicID, ID: mels})
	melsAfter := listContactsWith(t, env, melsToken)
	melsViewAfter := personalTokensOf(t, env, fixtures.VehementMemberMelEmail, fixtures.VehementPublicID)

	require.NoError(t, revoked)
	testhelper.ExternalRefusal(t, melsAfter, http.StatusUnauthorized)
	assert.Empty(t, melsViewAfter, "a revoked Personal token is no longer listed")
}

// A Personal token of a User with a required acceptance pending is refused like a Connection
// (ADR 0062): Mel has accepted none of the fixture terms.
func TestAPersonalTokenOfAGatedUserIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	res := listContactsWith(t, env, melsToken)

	refused := testhelper.ExternalRefusal(t, res, http.StatusForbidden)
	assert.Equal(t, externalapi.ProblemCodeAcceptancePending, refused.Code.Value)
}

// The Workspace's Two-factor requirement applies to a Personal token as to its User's session:
// once the User's grace has ended without a Second factor, the token is refused (ADR 0020).
func TestTheTwoFactorRequirementWithholdsAPersonalToken(t *testing.T) {
	// Umbrella requires a Second factor since 2026-03-01; Rita's grace ends a week later.
	graceEnds := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	at := graceEnds.Add(-time.Hour)
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
	inGrace := listContactsWith(t, env, ritasToken)
	require.NoError(t, inGrace)

	at = graceEnds.Add(time.Hour)
	res := listContactsWith(t, env, ritasToken)

	refused := testhelper.ExternalRefusal(t, res, http.StatusForbidden)
	assert.Equal(t, externalapi.ProblemCodeSecondFactorRequired, refused.Code.Value)
}
