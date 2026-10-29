package site_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/oauthcode"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteContactsErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)
	alice := idStr(fixtures.ContactAliceID)
	missing := idStr(99999)

	// Create: foreign workspace, then a duplicate alias key is a field-level 409.
	r1, err := c.SiteContactsCreate(ctx, &siteapi.SiteCreateContactInput{Email: siteapi.NewOptNilEmailAddress("new@example.com")}, siteapi.SiteContactsCreateParams{WorkspaceId: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactsCreateNotFound{}, r1)
	r2, err := c.SiteContactsCreate(ctx, &siteapi.SiteCreateContactInput{Email: siteapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail)}, siteapi.SiteContactsCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	conflict, ok := r2.(*siteapi.SiteContactsCreateConflict)
	require.Truef(t, ok, "got %T", r2)
	assert.Contains(t, conflict.Errors.Value, "email")

	// Get.
	g1, err := c.SiteContactsGet(ctx, siteapi.SiteContactsGetParams{WorkspaceId: foreign, ID: alice})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactsGetNotFound{}, g1)
	g3, err := c.SiteContactsGet(ctx, siteapi.SiteContactsGetParams{WorkspaceId: acme, ID: missing})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactsGetNotFound{}, g3)
	g4, err := c.SiteContactsGet(ctx, siteapi.SiteContactsGetParams{WorkspaceId: acme, ID: alice})
	require.NoError(t, err)
	got, ok := g4.(*siteapi.SiteContactResource)
	require.Truef(t, ok, "got %T", g4)
	assert.Equal(t, fixtures.ContactAliceEmail, string(got.Email.Or("")))

	// Update.
	rename := &siteapi.SiteUpdateContactInput{FirstName: siteapi.NewOptNilString("Renamed")}
	u1, err := c.SiteContactsUpdate(ctx, rename, siteapi.SiteContactsUpdateParams{WorkspaceId: foreign, ID: alice})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactsUpdateNotFound{}, u1)
	u3, err := c.SiteContactsUpdate(ctx, rename, siteapi.SiteContactsUpdateParams{WorkspaceId: acme, ID: missing})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactsUpdateNotFound{}, u3)
	u4, err := c.SiteContactsUpdate(ctx, &siteapi.SiteUpdateContactInput{Email: siteapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail)},
		siteapi.SiteContactsUpdateParams{WorkspaceId: acme, ID: idStr(fixtures.ContactBobID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactsUpdateConflict{}, u4, "bob cannot take alice's email")
	bob, err := env.DB.Contact.Get(ctx, fixtures.ContactBobID)
	require.NoError(t, err)
	assert.Equal(t, fixtures.ContactBobEmail, *bob.Email)

	// Delete.
	d1, err := c.SiteContactsDelete(ctx, siteapi.SiteContactsDeleteParams{WorkspaceId: foreign, ID: alice})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactsDeleteNotFound{}, d1)
	d2, err := c.SiteContactsDelete(ctx, siteapi.SiteContactsDeleteParams{WorkspaceId: acme, ID: missing})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactsDeleteNotFound{}, d2)
	stillThere, err := env.DB.Contact.Query().Where(contact.ID(fixtures.ContactAliceID)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, stillThere)
}

func TestSiteAnalyticsRangesAndEmptyWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// Globex has no delivery rows: every rate is 0 (zero denominators), and the
	// series is zero-filled for the whole window.
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	for _, tc := range []struct {
		r    siteapi.SiteAnalyticsRange
		days int
	}{
		{siteapi.SiteAnalyticsRange7d, 7},
		{siteapi.SiteAnalyticsRange30d, 30},
		{siteapi.SiteAnalyticsRange90d, 90},
	} {
		res, err := jane.SiteAnalyticsOverview(ctx, siteapi.SiteAnalyticsOverviewParams{
			WorkspaceId: fixtures.GlobexPublicID, Range: siteapi.NewOptSiteAnalyticsRange(tc.r),
		})
		require.NoError(t, err)
		ov, ok := res.(*siteapi.SiteAnalyticsOverview)
		require.Truef(t, ok, "got %T", res)
		assert.Len(t, ov.Timeseries, tc.days, string(tc.r))
		assert.Zero(t, ov.Email.SentCount)
		assert.Zero(t, ov.Email.OpenRate)
		assert.Zero(t, ov.Email.ClickRate)
		assert.Zero(t, ov.Email.ClickToOpenRate)
	}

	// No range given defaults to 7 days.
	def, err := jane.SiteAnalyticsOverview(ctx, siteapi.SiteAnalyticsOverviewParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
	assert.Len(t, def.(*siteapi.SiteAnalyticsOverview).Timeseries, 7)
}

func TestSiteOAuthConsentErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	const clientID, redirect = "fixture-client", "https://connector.example/callback"
	const verifier = "0123456789012345678901234567890123456789012"

	// Describe: unknown client 404, unregistered redirect 400, no usable scope 400.
	d1, err := owner.SiteOAuthDescribe(ctx, siteapi.SiteOAuthDescribeParams{ClientId: "nope", RedirectUri: redirect})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteOAuthDescribeNotFound{}, d1)
	d2, err := owner.SiteOAuthDescribe(ctx, siteapi.SiteOAuthDescribeParams{ClientId: clientID, RedirectUri: "https://evil.example/cb"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteOAuthDescribeBadRequest{}, d2)
	d3, err := owner.SiteOAuthDescribe(ctx, siteapi.SiteOAuthDescribeParams{ClientId: clientID, RedirectUri: redirect, Scope: siteapi.NewOptString("bogus")})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteOAuthDescribeBadRequest{}, d3)
	// Without a scope every grantable scope is offered; the send scopes list stays empty, not null.
	d4, err := owner.SiteOAuthDescribe(ctx, siteapi.SiteOAuthDescribeParams{ClientId: clientID, RedirectUri: redirect})
	require.NoError(t, err)
	req, ok := d4.(*siteapi.SiteOAuthAuthorizationRequest)
	require.Truef(t, ok, "got %T", d4)
	assert.NotEmpty(t, req.Scopes)
	assert.NotNil(t, req.SendScopes)
	assert.Empty(t, req.SendScopes)
	assert.NotNil(t, req.SetupScopes)
	assert.Empty(t, req.SetupScopes)

	in := func() *siteapi.SiteOAuthDecisionInput {
		return &siteapi.SiteOAuthDecisionInput{
			ClientId: clientID, RedirectUri: redirect, WorkspaceId: fixtures.AcmePublicID, Approve: true,
			CodeChallenge: oauth2.S256ChallengeFromVerifier(verifier),
		}
	}

	// Decide: foreign workspace, unknown client, bad redirect, bad PKCE, bad scope.
	foreignIn := in()
	foreignIn.WorkspaceId = fixtures.GlobexPublicID
	e1, err := owner.SiteOAuthDecide(ctx, foreignIn)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteOAuthDecideNotFound{}, e1)
	unknown := in()
	unknown.ClientId = "nope"
	e2, err := owner.SiteOAuthDecide(ctx, unknown)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteOAuthDecideNotFound{}, e2)
	badRedirect := in()
	badRedirect.RedirectUri = "https://evil.example/cb"
	e3, err := owner.SiteOAuthDecide(ctx, badRedirect)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteOAuthDecideBadRequest{}, e3)
	badPKCE := in()
	badPKCE.CodeChallenge = "short"
	e4, err := owner.SiteOAuthDecide(ctx, badPKCE)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteOAuthDecideBadRequest{}, e4)
	badScope := in()
	badScope.Scope = siteapi.NewOptString("bogus")
	e5, err := owner.SiteOAuthDecide(ctx, badScope)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteOAuthDecideBadRequest{}, e5)
	codes, err := env.DB.OAuthCode.Query().Where(oauthcode.WorkspaceID(fixtures.AcmeID)).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, codes, "rejected decisions mint no code")

	// A denial redirects with access_denied and mints no code.
	deny := in()
	deny.Approve = false
	deny.State = siteapi.NewOptString("st")
	e6, err := owner.SiteOAuthDecide(ctx, deny)
	require.NoError(t, err)
	denied, ok := e6.(*siteapi.SiteOAuthDecisionResult)
	require.Truef(t, ok, "got %T", e6)
	u, err := url.Parse(denied.RedirectUrl)
	require.NoError(t, err)
	assert.Equal(t, "access_denied", u.Query().Get("error"))
	assert.Equal(t, "st", u.Query().Get("state"))

	// A plain member may not connect an application.
	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	e7, err := member.SiteOAuthDecide(ctx, in())
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteOAuthDecideForbidden{}, e7)
}
