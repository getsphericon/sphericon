package site_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/consentrecord"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/consentrecords"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The optional consents in account settings (ADR 0062). MemberMary signed up answering the fixture
// items: the news consent declined. OwnerJohn signed up before any item existed and has no record.
// With the fixture terms in effect both first accept their current version, which the acceptance
// gate asks before anything else.

// withSignupItemsAccepted puts the fixture items in effect and has Mary and John accept the terms.
func withSignupItemsAccepted(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	withSignupItems(t, env)
	acceptTermsAs(t, env, fixtures.MemberMaryEmail, fixtures.OwnerJohnEmail)
}

// optionalConsents is the User's optional consents, each as its key and current decision ("none"
// with no answer).
func optionalConsents(t *testing.T, site *siteapi.Client) []string {
	t.Helper()
	res, err := site.SiteOptionalConsentsList(t.Context())
	require.NoError(t, err)
	return lo.Map(res.Items, func(c siteapi.SiteOptionalConsent, _ int) string {
		return string(c.Key) + ": " + string(c.Decision.Or("none"))
	})
}

func setOptionalConsent(t *testing.T, site *siteapi.Client, key, version string, accepted bool) (*siteapi.SiteOptionalConsent, error) {
	t.Helper()
	return site.SiteOptionalConsentsSet(t.Context(), &siteapi.SiteOptionalConsentChange{
		Version: siteapi.LegalDocumentLabel(version), Accepted: accepted,
	}, siteapi.SiteOptionalConsentsSetParams{Key: siteapi.SignupItemKey(key)})
}

func TestTheReadListsEachOptionalConsentInEffectWithTheUsersCurrentAnswer(t *testing.T) {
	env := testhelper.Setup(t)
	withSignupItemsAccepted(t, env)

	mary := optionalConsents(t, env.SiteActor(t, fixtures.MemberMaryEmail))
	john := optionalConsents(t, env.SiteActor(t, fixtures.OwnerJohnEmail))

	assert.Equal(t, []string{fixtures.OperatorNewsKey + ": declined"}, mary)
	assert.Equal(t, []string{fixtures.OperatorNewsKey + ": none"}, john)
}

func TestTheReadShowsTheCurrentVersionAndWhenTheAnswerWasGiven(t *testing.T) {
	env := testhelper.Setup(t)
	withSignupItemsAccepted(t, env)

	res, err := env.SiteActor(t, fixtures.MemberMaryEmail).SiteOptionalConsentsList(t.Context())
	require.NoError(t, err)

	require.Len(t, res.Items, 1)
	news := res.Items[0]
	assert.Equal(t, "News from us", news.Title)
	assert.Equal(t, siteapi.LegalDocumentLabel(fixtures.OperatorNewsCurrentLabel), news.Version)
	decidedOn := time.Time(news.DecidedAt.Value).UTC().Format(time.DateOnly)
	assert.Equal(t, "2026-01-05", decidedOn)
}

func TestWithNoOptionalConsentInEffectTheReadIsEmpty(t *testing.T) {
	env := testhelper.Setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID, fixtures.DataNoticeID)
	acceptTermsAs(t, env, fixtures.MemberMaryEmail)

	consents := optionalConsents(t, env.SiteActor(t, fixtures.MemberMaryEmail))

	assert.Empty(t, consents)
}

func TestGivingAndWithdrawingAConsentEachChangeTheCurrentAnswer(t *testing.T) {
	env := testhelper.Setup(t)
	withSignupItemsAccepted(t, env)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)

	given, err := setOptionalConsent(t, mary, fixtures.OperatorNewsKey, fixtures.OperatorNewsCurrentLabel, true)
	require.NoError(t, err)
	afterGiving := optionalConsents(t, mary)
	_, err = setOptionalConsent(t, mary, fixtures.OperatorNewsKey, fixtures.OperatorNewsCurrentLabel, false)
	require.NoError(t, err)
	afterWithdrawing := optionalConsents(t, mary)

	assert.Equal(t, siteapi.SiteOptionalConsentDecisionAccepted, given.Decision.Value)
	assert.Equal(t, []string{fixtures.OperatorNewsKey + ": accepted"}, afterGiving)
	assert.Equal(t, []string{fixtures.OperatorNewsKey + ": withdrawn"}, afterWithdrawing)
}

func TestAChangeIsRecordedWithTheSourceAccountSettings(t *testing.T) {
	env := testhelper.Setup(t)
	withSignupItemsAccepted(t, env)

	_, err := setOptionalConsent(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.OperatorNewsKey, fixtures.OperatorNewsCurrentLabel, true)
	require.NoError(t, err)

	records, err := env.ConsentRecords.Current(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	news, ok := lo.Find(records, func(r consentrecords.Record) bool { return r.Key == fixtures.OperatorNewsKey })
	require.True(t, ok)
	assert.Equal(t, consentrecord.SourceAccountSettings, news.Source)
	assert.Equal(t, consentrecord.DecisionAccepted, news.Decision)
}

func TestARequiredAcceptanceOrANoticeCannotBeChangedInAccountSettings(t *testing.T) {
	env := testhelper.Setup(t)
	withSignupItemsAccepted(t, env)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)
	before := currentAnswers(t, env, fixtures.MemberMaryID)
	cases := map[string][2]string{
		"required acceptance": {fixtures.ServiceTermsKey, fixtures.ServiceTermsCurrentLabel},
		"notice":              {fixtures.DataNoticeKey, fixtures.DataNoticeCurrentLabel},
	}
	for name, item := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := setOptionalConsent(t, mary, item[0], item[1], false)

			testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
			assert.Equal(t, before, currentAnswers(t, env, fixtures.MemberMaryID))
		})
	}
}

func TestAChangeOnAStaleVersionIsRefusedAndRecordsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	withSignupItemsAccepted(t, env)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)

	_, err := setOptionalConsent(t, mary, fixtures.OperatorNewsKey, "an-older-version", true)

	problem := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, []string{"version"}, lo.Keys(problem.Errors.Value))
	assert.Equal(t, []string{fixtures.OperatorNewsKey + ": declined"}, optionalConsents(t, mary))
}

func TestAConsentNotInEffectIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	acceptTermsAs(t, env, fixtures.MemberMaryEmail)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)
	cases := map[string]string{
		"deleted": fixtures.OperatorNewsKey,
		"unknown": "no-such-item",
	}
	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := setOptionalConsent(t, mary, key, fixtures.OperatorNewsCurrentLabel, true)

			testhelper.SiteRefusal(t, err, http.StatusNotFound)
		})
	}
}
