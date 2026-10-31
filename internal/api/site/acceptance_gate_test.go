package site_test

import (
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/consentrecord"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The acceptance gate on /site (ADR 0062). With the fixture terms in effect, every fixture User
// lacks an accepted record of their current version: MemberMary accepted the stale one when she
// signed up, the others signed up before any item existed.

// gated is the answer to a User with an acceptance pending; termsAccepted names the terms in effect.
var (
	gated         = outcome{http.StatusForbidden, string(siteapi.ProblemCodeAcceptancePending)}
	termsAccepted = siteapi.SiteAcceptance{Key: fixtures.ServiceTermsKey, Version: fixtures.ServiceTermsCurrentLabel}
)

// acceptPending accepts the pending acceptances named, as the screen does.
func acceptPending(t *testing.T, c *siteapi.Client, items ...siteapi.SiteAcceptance) siteapi.SiteUserAcceptPendingRes {
	t.Helper()
	res, err := c.SiteUserAcceptPending(t.Context(), &siteapi.SiteAcceptPendingInput{Items: items})
	require.NoError(t, err)
	return res
}

// acceptTermsAs has each of the fixture Users accept the terms in effect, so the gate lets them by.
func acceptTermsAs(t *testing.T, env *testhelper.TestEnv, emails ...string) {
	t.Helper()
	for _, email := range emails {
		res := acceptPending(t, env.SiteActor(t, email), termsAccepted)
		require.IsTypef(t, &siteapi.SiteUserResource{}, res, "%s accepts the terms", email)
	}
}

// pendingKeys is the keys of the acceptances getMe lists as pending for the User.
func pendingKeys(t *testing.T, c *siteapi.Client) []string {
	t.Helper()
	me, err := c.SiteUserGetMe(t.Context())
	require.NoError(t, err)
	return lo.Map(me.AcceptancesPending, func(i siteapi.SiteSignupItem, _ int) string { return string(i.Key) })
}

func TestAUserWhoHasNotAcceptedTheCurrentVersionIsRefusedWithAcceptancePending(t *testing.T) {
	env := testhelper.Setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	workspaces := send(t, env, fixtures.MemberMaryEmail, http.MethodGet, "/site/workspaces", "")
	tags := send(t, env, fixtures.MemberMaryEmail, http.MethodGet, acmePath("/tags"), "")
	optional := send(t, env, fixtures.OwnerJohnEmail, http.MethodGet, "/site/me/optional-consents", "")

	assert.Equal(t, gated, workspaces)
	assert.Equal(t, gated, tags)
	assert.Equal(t, gated, optional)
}

func TestTheOperationsAllowedWhileAnAcceptanceIsPendingAnswer(t *testing.T) {
	env := testhelper.Setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	mary := fixtures.MemberMaryEmail

	me := send(t, env, mary, http.MethodGet, "/site/me", "")
	secondFactor := send(t, env, mary, http.MethodGet, "/site/me/second-factor", "")
	deleted := send(t, env, mary, http.MethodGet, "/site/workspaces/deleted", "")
	document := send(t, env, mary, http.MethodGet, "/site/legal-documents/"+fixtures.ServiceTermsKey, "")
	items := send(t, env, mary, http.MethodGet, "/site/signup-items", "")

	assert.Equal(t, ok, me)
	assert.Equal(t, ok, secondFactor)
	assert.Equal(t, ok, deleted)
	assert.Equal(t, ok, document)
	assert.Equal(t, ok, items)
}

func TestTheProfileListsThePendingAcceptancesAtTheirCurrentVersion(t *testing.T) {
	env := testhelper.Setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	me, err := env.SiteActor(t, fixtures.MemberMaryEmail).SiteUserGetMe(t.Context())
	require.NoError(t, err)

	require.Len(t, me.AcceptancesPending, 1)
	assert.Equal(t, siteapi.SignupItemKey(fixtures.ServiceTermsKey), me.AcceptancesPending[0].Key)
	assert.Equal(t, siteapi.LegalDocumentLabel(fixtures.ServiceTermsCurrentLabel), me.AcceptancesPending[0].Version)
	assert.Equal(t, "Terms of service", me.AcceptancesPending[0].Title)
}

func TestAcceptingLiftsTheGateOnTheNextRequest(t *testing.T) {
	env := testhelper.Setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)

	res := acceptPending(t, mary, termsAccepted)
	after := send(t, env, fixtures.MemberMaryEmail, http.MethodGet, "/site/workspaces", "")

	accepted, isProfile := res.(*siteapi.SiteUserResource)
	require.Truef(t, isProfile, "got %T", res)
	assert.Empty(t, accepted.AcceptancesPending)
	assert.Equal(t, ok, after)
}

func TestAcceptingRecordsTheCurrentVersionAcceptedWithTheSourceReAcceptance(t *testing.T) {
	env := testhelper.Setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	acceptPending(t, env.SiteActor(t, fixtures.MemberMaryEmail), termsAccepted)

	records, err := env.ConsentRecords.Current(t.Context(), fixtures.MemberMaryID)
	require.NoError(t, err)
	terms, found := lo.Find(records, func(r consentrecords.Record) bool { return r.Key == fixtures.ServiceTermsKey })
	require.True(t, found)
	assert.Equal(t, fixtures.ServiceTermsCurrentLabel, terms.Version)
	assert.Equal(t, consentrecord.DecisionAccepted, terms.Decision)
	assert.Equal(t, consentrecord.SourceReAcceptance, terms.Source)
}

func TestAcceptingAStaleOrMissingVersionIsRefusedAndRecordsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)
	before := currentAnswers(t, env, fixtures.MemberMaryID)
	cases := map[string][]siteapi.SiteAcceptance{
		"stale version": {{Key: fixtures.ServiceTermsKey, Version: fixtures.ServiceTermsStaleLabel}},
		"none named":    {},
	}
	for name, items := range cases {
		t.Run(name, func(t *testing.T) {
			res := acceptPending(t, mary, items...)
			after := currentAnswers(t, env, fixtures.MemberMaryID)
			pending := pendingKeys(t, mary)

			assert.IsType(t, &siteapi.SiteUserAcceptPendingUnprocessableEntity{}, res)
			assert.Equal(t, before, after)
			assert.Equal(t, []string{fixtures.ServiceTermsKey}, pending)
		})
	}
}

// MemberMary accepted the fixture terms at their stale version when she signed up; the current
// version was published after.
func TestANewVersionGatesAUserWhoAcceptedThePreviousOne(t *testing.T) {
	env := testhelper.Setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	records, err := env.ConsentRecords.Current(t.Context(), fixtures.MemberMaryID)
	require.NoError(t, err)
	refused := send(t, env, fixtures.MemberMaryEmail, http.MethodGet, "/site/workspaces", "")
	pending := pendingKeys(t, env.SiteActor(t, fixtures.MemberMaryEmail))

	terms, found := lo.Find(records, func(r consentrecords.Record) bool { return r.Key == fixtures.ServiceTermsKey })
	require.True(t, found)
	assert.Equal(t, fixtures.ServiceTermsStaleLabel, terms.Version)
	assert.Equal(t, consentrecord.DecisionAccepted, terms.Decision)
	assert.Equal(t, gated, refused)
	assert.Equal(t, []string{fixtures.ServiceTermsKey}, pending)
}

func TestANewVersionOfAnOptionalConsentOrANoticeGatesNoOne(t *testing.T) {
	env := testhelper.Setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.OperatorNewsID, fixtures.DataNoticeID)

	workspaces := send(t, env, fixtures.MemberMaryEmail, http.MethodGet, "/site/workspaces", "")
	john := send(t, env, fixtures.OwnerJohnEmail, http.MethodGet, acmePath("/tags"), "")

	assert.Equal(t, ok, workspaces)
	assert.Equal(t, ok, john)
}

func TestARequiredItemAddedAfterUsersExistGatesThem(t *testing.T) {
	env := testhelper.Setup(t)
	before := send(t, env, fixtures.OwnerJohnEmail, http.MethodGet, "/site/workspaces", "")

	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	after := send(t, env, fixtures.OwnerJohnEmail, http.MethodGet, "/site/workspaces", "")

	assert.Equal(t, ok, before)
	assert.Equal(t, gated, after)
}

func TestAnOwnerMadeByTheOperatorIsGated(t *testing.T) {
	env := testhelper.Setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	_, err := accounts.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords).CreateOwner(t.Context(), "operator@example.com", "Ada Lovelace")
	require.NoError(t, err)

	refused := send(t, env, "operator@example.com", http.MethodGet, "/site/workspaces", "")

	assert.Equal(t, gated, refused)
}

func TestAGatedUserDeletesTheirAccount(t *testing.T) {
	env := testhelper.Setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	res := deleteUser(t, env.SiteActor(t, fixtures.MemberMaryEmail), &siteapi.SiteDeleteUserInput{
		Email:           fixtures.MemberMaryEmail,
		CurrentPassword: siteapi.NewOptString(fixtures.MemberMaryPassword),
	})

	exists := userExists(t, env, fixtures.MemberMaryEmail)

	assert.IsType(t, &siteapi.SiteUserDeleteUserNoContent{}, res)
	assert.False(t, exists)
}

// A gated sole Owner is not trapped: the deletion of their account names the Workspaces they
// solely own, they delete each, and then their account.
func TestAGatedSoleOwnerDeletesTheirWorkspacesThenTheirAccount(t *testing.T) {
	env := testhelper.Setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	dora := env.SiteActor(t, fixtures.SoleOwnerDoraEmail)
	input := &siteapi.SiteDeleteUserInput{Email: fixtures.SoleOwnerDoraEmail, CurrentPassword: siteapi.NewOptString(fixtures.SoleOwnerDoraPassword)}

	refused := deleteUser(t, dora, input)
	piedPiper := deleteWorkspace(t, dora, fixtures.PiedPiperPublicID, fixtures.PiedPiperName)
	aviato := deleteWorkspace(t, dora, fixtures.AviatoPublicID, fixtures.AviatoName)
	deleted := deleteUser(t, dora, input)
	exists := userExists(t, env, fixtures.SoleOwnerDoraEmail)

	assert.IsType(t, &siteapi.SiteDeleteUserProblemDetails{}, refused)
	assert.IsType(t, &siteapi.SiteDeletedWorkspaceResource{}, piedPiper)
	assert.IsType(t, &siteapi.SiteDeletedWorkspaceResource{}, aviato)
	assert.IsType(t, &siteapi.SiteUserDeleteUserNoContent{}, deleted)
	assert.False(t, exists)
}

func TestAGatedUserCannotAcceptAnInvitation(t *testing.T) {
	env := testhelper.Setup(t)
	token := inviteToAcme(t, env, fixtures.OutsiderOscarEmail)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	refused := send(t, env, fixtures.OutsiderOscarEmail, http.MethodPost, "/site/invitations/"+token+"/accept", "{}")
	member := isMemberOfAcme(t, env, fixtures.OutsiderOscarEmail)

	assert.Equal(t, gated, refused)
	assert.False(t, member)
}
