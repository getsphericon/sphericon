package site_test

import (
	"context"
	"net/http"
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
	_, err := c.SiteContactsCreate(ctx, &siteapi.SiteCreateContactInput{Email: siteapi.NewOptNilEmailAddress("new@example.com")}, siteapi.SiteContactsCreateParams{WorkspaceId: foreign})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteContactsCreate(ctx, &siteapi.SiteCreateContactInput{Email: siteapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail)}, siteapi.SiteContactsCreateParams{WorkspaceId: acme})
	conflict := testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Contains(t, conflict.Errors.Value, "email")

	// Get.
	_, err = c.SiteContactsGet(ctx, siteapi.SiteContactsGetParams{WorkspaceId: foreign, ID: alice})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteContactsGet(ctx, siteapi.SiteContactsGetParams{WorkspaceId: acme, ID: missing})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	g4, err := c.SiteContactsGet(ctx, siteapi.SiteContactsGetParams{WorkspaceId: acme, ID: alice})
	require.NoError(t, err)
	got := g4
	assert.Equal(t, fixtures.ContactAliceEmail, string(got.Email.Or("")))

	// Update.
	rename := &siteapi.SiteUpdateContactInput{Revision: g4.Revision, FirstName: siteapi.NewOptNilString("Renamed")}
	_, err = c.SiteContactsUpdate(ctx, rename, siteapi.SiteContactsUpdateParams{WorkspaceId: foreign, ID: alice})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteContactsUpdate(ctx, rename, siteapi.SiteContactsUpdateParams{WorkspaceId: acme, ID: missing})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteContactsUpdate(ctx, &siteapi.SiteUpdateContactInput{Revision: siteapi.Revision(env.ContactRevision(ctx, t, int64(idStr(fixtures.ContactBobID)))), Email: siteapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail)},
		siteapi.SiteContactsUpdateParams{WorkspaceId: acme, ID: idStr(fixtures.ContactBobID)})
	testhelper.SiteRefusal(t, err, http.StatusConflict, "bob cannot take alice's email")
	bob, err := env.DB.Contact.Get(ctx, fixtures.ContactBobID)
	require.NoError(t, err)
	assert.Equal(t, fixtures.ContactBobEmail, *bob.Email)

	// Delete.
	err = c.SiteContactsDelete(ctx, siteapi.SiteContactsDeleteParams{WorkspaceId: foreign, ID: alice})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	err = c.SiteContactsDelete(ctx, siteapi.SiteContactsDeleteParams{WorkspaceId: acme, ID: missing})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
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
		ov := res
		assert.Len(t, ov.Timeseries, tc.days, string(tc.r))
		// Globex's one marketing message (a usage fixture, sent an hour ago) is in every cohort, unengaged.
		assert.Equal(t, int32(1), ov.Current.Email.SentCount)
		assert.Zero(t, ov.Current.Email.OpenedCount)
		assert.Zero(t, ov.Current.Email.OpenRate)
		assert.Zero(t, ov.Current.Email.ClickRate)
		assert.Zero(t, ov.Current.Email.ClickToOpenRate)
	}

	// No range given defaults to 7 days.
	def, err := jane.SiteAnalyticsOverview(ctx, siteapi.SiteAnalyticsOverviewParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
	assert.Len(t, def.Timeseries, 7)
}

func TestSiteOAuthConsentErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	const clientID, redirect = "fixture-client", "https://connector.example/callback"
	const verifier = "0123456789012345678901234567890123456789012"

	// Describe: unknown client 404, unregistered redirect 422, no usable scope 422.
	_, err := owner.SiteOAuthDescribe(ctx, siteapi.SiteOAuthDescribeParams{ClientId: "nope", RedirectUri: redirect})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = owner.SiteOAuthDescribe(ctx, siteapi.SiteOAuthDescribeParams{ClientId: clientID, RedirectUri: "https://evil.example/cb"})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	_, err = owner.SiteOAuthDescribe(ctx, siteapi.SiteOAuthDescribeParams{ClientId: clientID, RedirectUri: redirect, Scope: siteapi.NewOptString("bogus")})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	// Without a scope every grantable scope is offered; the send scopes list stays empty, not null.
	d4, err := owner.SiteOAuthDescribe(ctx, siteapi.SiteOAuthDescribeParams{ClientId: clientID, RedirectUri: redirect})
	require.NoError(t, err)
	req := d4
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
	_, err = owner.SiteOAuthDecide(ctx, foreignIn)
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	unknown := in()
	unknown.ClientId = "nope"
	_, err = owner.SiteOAuthDecide(ctx, unknown)
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	badRedirect := in()
	badRedirect.RedirectUri = "https://evil.example/cb"
	_, err = owner.SiteOAuthDecide(ctx, badRedirect)
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	badPKCE := in()
	badPKCE.CodeChallenge = "short"
	_, err = owner.SiteOAuthDecide(ctx, badPKCE)
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	badScope := in()
	badScope.Scope = siteapi.NewOptString("bogus")
	_, err = owner.SiteOAuthDecide(ctx, badScope)
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	codes, err := env.DB.OAuthCode.Query().Where(oauthcode.WorkspaceID(fixtures.AcmeID), oauthcode.IDNEQ(fixtures.OAuthCodeAcmeID)).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, codes, "rejected decisions mint no code")

	// A denial redirects with access_denied and mints no code.
	deny := in()
	deny.Approve = false
	deny.State = siteapi.NewOptString("st")
	e6, err := owner.SiteOAuthDecide(ctx, deny)
	require.NoError(t, err)
	denied := e6
	u, err := url.Parse(denied.RedirectUrl)
	require.NoError(t, err)
	assert.Equal(t, "access_denied", u.Query().Get("error"))
	assert.Equal(t, "st", u.Query().Get("state"))

	// A plain member may not connect an application.
	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	_, err = member.SiteOAuthDecide(ctx, in())
	testhelper.SiteRefusal(t, err, http.StatusForbidden)
}
