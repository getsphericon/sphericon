package oauth_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/ent/oauthcode"
	"github.com/getsphericon/sphericon/ent/suppression"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// connectAs plays the consent of the User email to scope in the Workspace workspaceID and the
// code exchange, and returns the Connection's first token response.
func connectAs(t *testing.T, env *testhelper.TestEnv, email string, workspaceID siteapi.WorkspaceId, scope string) tokenResponse {
	t.Helper()
	in := approval(scope, false)
	in.WorkspaceId = workspaceID
	back := consentAs(t, env, email, in)
	rec := exchange(t, env, tokenForm(back.Query().Get("code")))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decodeToken(t, rec)
}

// A member may approve a client (ADR 0061): the Connection is hers, listed among her Connections
// and not among the Workspace's API tokens.
func TestAMemberMakesAConnectionOfHerOwn(t *testing.T) {
	env := testhelper.Setup(t)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)

	tok := connectAs(t, env, fixtures.MemberMaryEmail, fixtures.AcmePublicID, "contacts:read contacts:write")

	assert.Equal(t, "contacts:read contacts:write", tok.Scope)
	row := connectionOf(t, env, tok.AccessToken)
	assert.Equal(t, new(int64(fixtures.AcmeMemberMembershipID)), row.MembershipID)
	_, err := env.MCPClient(t, tok.AccessToken).ListTools(t.Context(), nil)
	require.NoError(t, err)

	listed, err := mary.SiteConnectionsList(t.Context(), siteapi.SiteConnectionsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	connections := listed
	require.Len(t, connections, 1)
	assert.Equal(t, siteapi.EntityId(row.ID), connections[0].ID)
	assert.Equal(t, "Fixture Connector", connections[0].Name)
	assert.Equal(t, fixtures.MemberMaryName, connections[0].UserName)
	assert.Equal(t, siteapi.EntityId(fixtures.MemberMaryID), connections[0].UserId)

	tokens, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteTokensList(t.Context(), siteapi.SiteTokensListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	for _, token := range tokens {
		assert.NotEqual(t, siteapi.EntityId(row.ID), token.ID, "a Connection is not shown with the API tokens")
	}
	personalTokens, err := mary.SitePersonalTokensList(t.Context(), siteapi.SitePersonalTokensListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.Empty(t, personalTokens, "a Connection is not a Personal token")
}

// The consent screen learns what the User may grant, and the server refuses a scope whose
// Permission the User's Role lacks: no code is issued.
func TestConsentBoundsTheScopesByTheUsersRole(t *testing.T) {
	env := testhelper.Setup(t)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)

	described, err := mary.SiteOAuthDescribe(t.Context(), siteapi.SiteOAuthDescribeParams{
		ClientId: fixtureClientID, RedirectUri: fixtureRedirectURI,
		Scope: siteapi.NewOptString("contacts:read webhooks:read contacts:export suppressions:write"),
	})
	require.NoError(t, err)
	assert.Equal(t, siteapi.SiteOAuthAuthorizationRequestRequiredPermissions{
		"webhooks:read":      siteapi.PermissionWebhooksManage,
		"contacts:export":    siteapi.PermissionContactsExport,
		"suppressions:write": siteapi.PermissionSuppressionsManage,
	}, described.RequiredPermissions)

	for name, in := range map[string]siteapi.SiteOAuthDecisionInput{
		"a granted scope":      approval("contacts:read webhooks:read", false),
		"exporting a Contact":  approval("contacts:read contacts:export", false),
		"adding a Suppression": approval("contacts:write suppressions:write", false),
		"a setup scope": func() siteapi.SiteOAuthDecisionInput {
			in := approval("contacts:read sending_domains:write", false)
			in.AllowSetup = siteapi.NewOptBool(true)
			return in
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := mary.SiteOAuthDecide(t.Context(), &in)

			refused := testhelper.SiteRefusal(t, err, http.StatusForbidden)
			assert.Equal(t, siteapi.ProblemCodePermissionRequired, refused.Code.Value)
			issued, err := env.DB.OAuthCode.Query().Where(oauthcode.MembershipID(fixtures.AcmeMemberMembershipID)).Exist(t.Context())
			require.NoError(t, err)
			assert.False(t, issued, "a refused consent issues no code")
		})
	}
}

// demoteVehementAdmin makes the Vehement admin Ada a member.
func demoteVehementAdmin(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	_, err := env.SiteActor(t, fixtures.VehementOwnerValEmail).SiteMembershipsUpdate(t.Context(),
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleMember},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.VehementPublicID, ID: siteapi.EntityId(fixtures.VehementAdminMembershipID)})
	require.NoError(t, err)
}

// A Connection can do what its scopes and its User's current Role allow: an admin's Connection to
// the webhook endpoints loses them on the request after the admin is made a member.
func TestADemotionTakesEffectOnTheNextRequest(t *testing.T) {
	env := testhelper.Setup(t)
	api := env.ExternalWithToken(t, adasConnection)

	_, err := api.WebhooksList(t.Context(), externalapi.WebhooksListParams{})
	require.NoError(t, err)

	demoteVehementAdmin(t, env)

	_, err = api.WebhooksList(t.Context(), externalapi.WebhooksListParams{})
	refused := testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	assert.Equal(t, externalapi.ProblemCodePermissionRequired, refused.Code.Value)
}

// An owner and an admin may grant exporting a Contact and adding a Suppression, which their Roles
// hold, and their Connection does both.
func TestAnOwnerOrAdminGrantsExportAndSuppressions(t *testing.T) {
	for name, email := range map[string]string{"owner": fixtures.OwnerJohnEmail, "admin": fixtures.AdminAliceEmail} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			tok := connectAs(t, env, email, fixtures.AcmePublicID, "contacts:export suppressions:write")
			assert.Equal(t, "contacts:export suppressions:write", tok.Scope)
			api := env.ExternalWithToken(t, tok.AccessToken)

			exported, err := api.ContactsExport(t.Context(), externalapi.ContactsExportParams{Email: externalapi.NewOptEmailAddress(fixtures.ContactBobEmail)})
			require.NoError(t, err)
			assert.IsType(t, &externalapi.ContactsExportOKApplicationOctetStreamHeaders{}, exported)

			_, err = api.SuppressionsCreate(t.Context(), &externalapi.CreateSuppressionInput{Destination: "bounded@example.com"})
			require.NoError(t, err)
		})
	}
}

// The Role bounds the new scopes on every request (ADR 0061): an admin's Connection that exports
// and suppresses answers 403 permission_required on both once the admin is made a member, and the
// refused write leaves no row.
func TestADemotionTakesExportAndSuppressionsAway(t *testing.T) {
	env := testhelper.Setup(t)
	api := env.ExternalWithToken(t, adasConnection)

	demoteVehementAdmin(t, env)

	_, err := api.ContactsExport(t.Context(), externalapi.ContactsExportParams{ID: externalapi.NewOptEntityId(externalapi.EntityId(fixtures.ContactVehementVicID))})
	refusedExport := testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	assert.Equal(t, externalapi.ProblemCodePermissionRequired, refusedExport.Code.Value)

	_, err = api.SuppressionsCreate(t.Context(), &externalapi.CreateSuppressionInput{Destination: "bounded@example.com"})
	refusedSuppression := testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	assert.Equal(t, externalapi.ProblemCodePermissionRequired, refusedSuppression.Code.Value)
	added, err := env.DB.Suppression.Query().Where(suppression.Destination("bounded@example.com")).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, added, "the refused call suppressed nothing")
}

// assertMelsConnectionEnded checks that Mel's Connection is over: its access token is refused on
// /mcp and /api, its refresh token is no grant, and no row of her Membership is left.
func assertMelsConnectionEnded(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	assert.Equal(t, http.StatusUnauthorized, mcpStatus(t, env, melsConnection))
	_, err := env.ExternalWithToken(t, melsConnection).ContactsList(t.Context(), externalapi.ContactsListParams{})
	testhelper.ExternalRefusal(t, err, http.StatusUnauthorized)
	assert.Equal(t, "invalid_grant", decodeToken(t, exchange(t, env, refreshForm(fixtures.VehementMemberRefreshToken))).Error)
	remaining, err := env.DB.ApiToken.Query().Where(apitoken.MembershipID(fixtures.VehementMemberMembershipID)).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, remaining)
}

// A Connection ends with its Membership: Leaving the Workspace removes it and its token is
// refused from then on.
func TestLeavingEndsTheUsersConnections(t *testing.T) {
	env := testhelper.Setup(t)
	require.NotEqual(t, http.StatusUnauthorized, mcpStatus(t, env, melsConnection))

	err := env.SiteActor(t, fixtures.VehementMemberMelEmail).SiteWorkspacesLeave(t.Context(), siteapi.SiteWorkspacesLeaveParams{WorkspaceId: fixtures.VehementPublicID})
	require.NoError(t, err)

	assertMelsConnectionEnded(t, env)
}

// A Connection ends when an Owner or Admin removes its User's Membership.
func TestRemovalEndsTheUsersConnections(t *testing.T) {
	env := testhelper.Setup(t)
	require.NotEqual(t, http.StatusUnauthorized, mcpStatus(t, env, melsConnection))

	err := env.SiteActor(t, fixtures.VehementAdminAdaEmail).SiteMembershipsDelete(t.Context(),
		siteapi.SiteMembershipsDeleteParams{WorkspaceId: fixtures.VehementPublicID, ID: siteapi.EntityId(fixtures.VehementMemberMembershipID)})
	require.NoError(t, err)

	assertMelsConnectionEnded(t, env)
}

// A Connection ends when its User deletes themself.
func TestUserDeletionEndsTheUsersConnections(t *testing.T) {
	env := testhelper.Setup(t)
	require.NotEqual(t, http.StatusUnauthorized, mcpStatus(t, env, melsConnection))

	_, err := env.SiteActor(t, fixtures.VehementMemberMelEmail).SiteUserDeleteUser(t.Context(), &siteapi.SiteDeleteUserInput{
		Email:           fixtures.VehementMemberMelEmail,
		CurrentPassword: siteapi.NewOptString(fixtures.VehementMemberMelPassword),
	})
	require.NoError(t, err)

	assertMelsConnectionEnded(t, env)
}

// What a Connection does is recorded in the Audit log as done by its User, named through the
// client the User approved (ADR 0061).
func TestAConnectionActsAsItsUser(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.ExternalWithToken(t, melsConnection).ContactsUpdate(t.Context(),
		&externalapi.UpdateContactInput{Revision: externalapi.Revision(env.ContactRevision(t.Context(), t, int64(externalapi.EntityId(fixtures.ContactVehementVicID)))), FirstName: externalapi.NewOptNilString("Renamed")},
		externalapi.ContactsUpdateParams{ID: externalapi.EntityId(fixtures.ContactVehementVicID)})
	require.NoError(t, err)

	got := env.QueuedEvents(t, events.NameAuditEntry)
	require.Len(t, got, 1)
	assert.Equal(t, events.Actor{Kind: events.ActorUser, ID: strconv.Itoa(fixtures.VehementMemberMelID), Name: fixtures.VehementMemberMelName + " via " + fixtures.VehementMemberConnectionName},
		got[0].(*events.AuditEntry).Actor)
}

// Each User sees and revokes their own Connections; members:manage sees and revokes anyone's.
func TestConnectionsAreTheirUsersOrAManagersToRevoke(t *testing.T) {
	env := testhelper.Setup(t)
	owners := siteapi.EntityId(fixtures.VehementOwnerConnectionID)
	mel := env.SiteActor(t, fixtures.VehementMemberMelEmail)
	params := siteapi.SiteConnectionsListParams{WorkspaceId: fixtures.VehementPublicID}
	connectionIDs := func(r siteapi.SiteConnectionResource, _ int) siteapi.EntityId { return r.ID }

	melList, err := mel.SiteConnectionsList(t.Context(), params)
	require.NoError(t, err)
	melSees := lo.Map(melList, connectionIDs)
	assert.Equal(t, []siteapi.EntityId{fixtures.VehementMemberConnectionID}, melSees, "a member sees only her own Connection")
	err = mel.SiteConnectionsDelete(t.Context(), siteapi.SiteConnectionsDeleteParams{WorkspaceId: fixtures.VehementPublicID, ID: owners})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	admin := env.SiteActor(t, fixtures.VehementAdminAdaEmail)
	adminList, err := admin.SiteConnectionsList(t.Context(), params)
	require.NoError(t, err)
	adminSees := lo.Map(adminList, connectionIDs)
	assert.ElementsMatch(t, []siteapi.EntityId{owners, fixtures.VehementAdminConnectionID, fixtures.VehementMemberConnectionID}, adminSees, "members:manage sees anyone's")
	err = admin.SiteConnectionsDelete(t.Context(), siteapi.SiteConnectionsDeleteParams{WorkspaceId: fixtures.VehementPublicID, ID: owners})
	require.NoError(t, err)
	assert.NotNil(t, connection(t, env, fixtures.VehementOwnerConnectionID).RevokedAt)
}

// The Workspace's Two-factor requirement applies to a Connection as to its User's session: once
// the User's grace has ended without a Second factor, the Connection is refused (ADR 0020).
func TestTheTwoFactorRequirementWithholdsAConnection(t *testing.T) {
	// Umbrella requires a Second factor since 2026-03-01; Rita's grace ends a week later.
	graceEnds := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	clock := &shiftingClock{at: graceEnds.Add(-time.Hour)}
	env := testhelper.Setup(t, testhelper.WithClock(clock.now))
	require.NotEqual(t, http.StatusForbidden, mcpStatus(t, env, ritasConnection))

	clock.at = graceEnds.Add(time.Hour)

	rec := serve(env, mcpRequest(t, ritasConnection))
	assert.Equal(t, http.StatusForbidden, rec.Code)
	var problem struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
	assert.Equal(t, string(siteapi.ProblemCodeSecondFactorRequired), problem.Code)
}

// shiftingClock is a clock a test moves forward.
type shiftingClock struct{ at time.Time }

func (c *shiftingClock) now() time.Time { return c.at }
