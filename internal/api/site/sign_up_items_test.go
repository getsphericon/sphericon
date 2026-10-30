package site_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/consentrecord"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Sign-up items on the password Sign-up (ADR 0062): the fixture items, a required acceptance,
// an optional consent and a notice, are put in effect by each test that asks them.

func withSignupItems(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID, fixtures.OperatorNewsID, fixtures.DataNoticeID)
}

func itemAnswer(key, label string, accepted bool) siteapi.SiteSignupItemAnswer {
	return siteapi.SiteSignupItemAnswer{Key: siteapi.SignupItemKey(key), Version: siteapi.LegalDocumentLabel(label), Accepted: siteapi.NewOptBool(accepted)}
}

var (
	termsTicked  = itemAnswer(fixtures.ServiceTermsKey, fixtures.ServiceTermsCurrentLabel, true)
	newsUnticked = itemAnswer(fixtures.OperatorNewsKey, fixtures.OperatorNewsCurrentLabel, false)
)

func signUpAnswering(t *testing.T, env *testhelper.TestEnv, email string, answers ...siteapi.SiteSignupItemAnswer) siteapi.SiteAuthSignUpRes {
	t.Helper()
	res, err := env.SiteAnonymous(t).SiteAuthSignUp(t.Context(), &siteapi.SiteSignUpInput{
		Name: "Grace Hopper", Email: siteapi.EmailAddress(email), Password: siteapi.NewOptString(formPassword), SignupItems: answers,
	})
	require.NoError(t, err)
	return res
}

// currentAnswers is the User's current answers, each as its key and decision.
func currentAnswers(t *testing.T, env *testhelper.TestEnv, userID int64) []string {
	t.Helper()
	records, err := env.ConsentRecords.Current(t.Context(), userID)
	require.NoError(t, err)
	return lo.Map(records, func(r consentrecords.Record, _ int) string { return r.Key + ": " + string(r.Decision) })
}

func TestASignUpWithoutARequiredAcceptanceIsRefusedWithAFieldNamingIt(t *testing.T) {
	env := testhelper.Setup(t)
	withSignupItems(t, env)

	res := signUpAnswering(t, env, "grace@example.com", newsUnticked)

	problem, ok := res.(*siteapi.SiteAuthSignUpUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.ProblemDetailsErrors{
		"signupItems." + fixtures.ServiceTermsKey: {"accept the Terms of service to sign up"},
	}, problem.Errors.Value)
	assert.Equal(t, "accept the Terms of service to sign up", problem.Detail.Value)
	assert.False(t, userExists(t, env, "grace@example.com"))
	assert.Empty(t, env.SystemMail.Messages())
}

func TestARefusedItemAndARefusedFieldAreNamedTogether(t *testing.T) {
	env := testhelper.Setup(t)
	withSignupItems(t, env)

	res := signUpWithPassword(t, env, " ", "grace@example.com", formPassword)

	problem, ok := res.(*siteapi.SiteAuthSignUpUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, []string{"name", "signupItems." + fixtures.ServiceTermsKey}, slices.Sorted(maps.Keys(problem.Errors.Value)))
	assert.Equal(t, "check the name and sign-up items", problem.Detail.Value)
}

func TestASignUpWithOptionalConsentsLeftUntickedSucceedsAndRecordsEveryAnswer(t *testing.T) {
	env := testhelper.Setup(t)
	withSignupItems(t, env)

	res := signUpAnswering(t, env, "grace@example.com", termsTicked, newsUnticked)

	require.IsType(t, &siteapi.SiteAuthSignUpAccepted{}, res)
	grace := userByEmail(t, env, "grace@example.com")
	assert.Equal(t, []string{
		fixtures.ServiceTermsKey + ": accepted",
		fixtures.OperatorNewsKey + ": declined",
		fixtures.DataNoticeKey + ": shown",
	}, currentAnswers(t, env, grace.ID))
}

func TestAnUnknownItemOrAStaleVersionIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	withSignupItems(t, env)
	cases := map[string]siteapi.SiteSignupItemAnswer{
		"unknown item":  itemAnswer("no-such-item", fixtures.ServiceTermsCurrentLabel, true),
		"stale version": itemAnswer(fixtures.ServiceTermsKey, fixtures.ServiceTermsStaleLabel, true),
	}
	for name, wrong := range cases {
		t.Run(name, func(t *testing.T) {
			res := signUpAnswering(t, env, "grace@example.com", termsTicked, wrong)

			problem, ok := res.(*siteapi.SiteAuthSignUpUnprocessableEntity)
			require.Truef(t, ok, "got %T", res)
			assert.Equal(t, []string{"signupItems"}, lo.Keys(problem.Errors.Value))
			assert.False(t, userExists(t, env, "grace@example.com"))
		})
	}
}

func TestWithNoSignupItemsASignUpAsksNothingAndRecordsNothing(t *testing.T) {
	env := testhelper.Setup(t)

	res := signUpAnswering(t, env, "grace@example.com")

	require.IsType(t, &siteapi.SiteAuthSignUpAccepted{}, res)
	grace := userByEmail(t, env, "grace@example.com")
	assert.Empty(t, currentAnswers(t, env, grace.ID))
}

// The record keeps the client's address as the rate limiter resolves it (the rightmost entry our
// one proxy appended) and the user agent.
func TestARecordKeepsTheClientAddressAndUserAgent(t *testing.T) {
	env := testhelper.Setup(t)
	withSignupItems(t, env)
	body, err := json.Marshal(siteapi.SiteSignUpInput{
		Name: "Grace Hopper", Email: "grace@example.com", Password: siteapi.NewOptString(formPassword),
		SignupItems: []siteapi.SiteSignupItemAnswer{termsTicked},
	})
	require.NoError(t, err)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/site/auth/sign-up", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "198.51.100.1, 203.0.113.7")
	req.Header.Set("User-Agent", "Grace's browser")
	rec := httptest.NewRecorder()

	env.Server.ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	records, err := env.ConsentRecords.Current(t.Context(), userByEmail(t, env, "grace@example.com").ID)
	require.NoError(t, err)
	terms, ok := lo.Find(records, func(r consentrecords.Record) bool { return r.Key == fixtures.ServiceTermsKey })
	require.True(t, ok)
	assert.Equal(t, consentrecord.SourceSignUpForm, terms.Source)
	assert.Equal(t, "203.0.113.7", terms.IP)
	assert.Equal(t, "Grace's browser", terms.UserAgent)
}

func TestDeletingTheAccountRemovesTheUsersConsentRecords(t *testing.T) {
	env := testhelper.Setup(t)
	before := currentAnswers(t, env, fixtures.MemberMaryID)

	res := deleteUser(t, env.SiteActor(t, fixtures.MemberMaryEmail), &siteapi.SiteDeleteUserInput{
		Email: fixtures.MemberMaryEmail, CurrentPassword: siteapi.NewOptString(fixtures.MemberMaryPassword),
	})

	require.IsType(t, &siteapi.SiteUserDeleteUserNoContent{}, res)
	assert.NotEmpty(t, before)
	assert.Empty(t, currentAnswers(t, env, fixtures.MemberMaryID))
}
