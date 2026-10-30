package signup_test

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/signup"
	"github.com/getsphericon/sphericon/internal/validation"
)

// withItems puts the fixture Sign-up items in effect: a required acceptance, an optional consent
// and a notice (ADR 0062).
func (r rig) withItems(t *testing.T) {
	t.Helper()
	r.env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID, fixtures.OperatorNewsID, fixtures.DataNoticeID)
}

// answer is the answer to a fixture item at the label given.
func answer(key, label string, accepted bool) consentrecords.Answer {
	return consentrecords.Answer{Key: key, Version: label, Accepted: accepted}
}

var (
	// termsAccepted is the required acceptance ticked at its current version.
	termsAccepted = answer(fixtures.ServiceTermsKey, fixtures.ServiceTermsCurrentLabel, true)
	newsTicked    = answer(fixtures.OperatorNewsKey, fixtures.OperatorNewsCurrentLabel, true)
	newsUnticked  = answer(fixtures.OperatorNewsKey, fixtures.OperatorNewsCurrentLabel, false)
)

// What the fixture items' current answers read as, by decision.
var (
	termsAcceptedRecord = fixtures.ServiceTermsKey + "@" + fixtures.ServiceTermsCurrentLabel + ": accepted from sign_up_form"
	newsAcceptedRecord  = fixtures.OperatorNewsKey + "@" + fixtures.OperatorNewsCurrentLabel + ": accepted from sign_up_form"
	newsDeclinedRecord  = fixtures.OperatorNewsKey + "@" + fixtures.OperatorNewsCurrentLabel + ": declined from sign_up_form"
	noticeShownRecord   = fixtures.DataNoticeKey + "@" + fixtures.DataNoticeCurrentLabel + ": shown from sign_up_form"
)

func (r rig) signUpAnswering(t *testing.T, email string, answers ...consentrecords.Answer) (signup.Result, error) {
	t.Helper()
	return r.module.SignUp(t.Context(), signup.Input{Name: "Grace", Email: email, Password: password, SignupItems: answers})
}

// decided is the User's current answers, each as its key, label, decision and source.
func (r rig) decided(t *testing.T, email string) []string {
	t.Helper()
	records, err := r.consents.Current(t.Context(), r.user(t, email).ID)
	require.NoError(t, err)
	return lo.Map(records, func(rec consentrecords.Record, _ int) string {
		return rec.Key + "@" + rec.Version + ": " + string(rec.Decision) + " from " + string(rec.Source)
	})
}

// refusedFields is the fields a refused Sign-up names.
func refusedFields(t *testing.T, err error) []string {
	t.Helper()
	fields, ok := errors.AsType[*validation.FieldsError](err)
	require.Truef(t, ok, "refused, got %v", err)
	return slices.Sorted(maps.Keys(fields.Fields))
}

// ADR 0062: the server refuses a Sign-up without a required acceptance, whatever the browser
// sent, before any work: no User, no budget slot, no mail.
func TestASignUpWithoutARequiredAcceptanceIsRefusedNamingItAndCostsNothing(t *testing.T) {
	r := newRig(t, 5)
	r.withItems(t)

	_, err := r.signUpAnswering(t, "grace@example.com", newsTicked)

	assert.Equal(t, []string{"signupItems." + fixtures.ServiceTermsKey}, refusedFields(t, err))
	exists, err := r.db.User.Query().Where(user.Email("grace@example.com")).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Empty(t, r.queue.jobs)
	assert.False(t, r.budgetTaken(t, "grace@example.com"))
}

func TestTheItemsAreRefusedTogetherWithTheOtherFields(t *testing.T) {
	r := newRig(t, 5)
	r.withItems(t)

	_, err := r.module.SignUp(t.Context(), signup.Input{Name: " ", Email: "grace@example.com", Password: password})

	assert.Equal(t, []string{"name", "signupItems." + fixtures.ServiceTermsKey}, refusedFields(t, err))
}

func TestASignUpRecordsEveryAnswerAndEveryNoticeShownOnTheNewUser(t *testing.T) {
	r := newRig(t, 5)
	r.withItems(t)

	res, err := r.signUpAnswering(t, "grace@example.com", termsAccepted, newsUnticked)

	require.NoError(t, err)
	assert.Equal(t, signup.Accepted, res.Outcome)
	assert.Equal(t, []string{termsAcceptedRecord, newsDeclinedRecord, noticeShownRecord}, r.decided(t, "grace@example.com"))
}

func TestAnOptionalConsentLeftOutOfTheAnswersIsNotRecorded(t *testing.T) {
	r := newRig(t, 5)
	r.withItems(t)

	_, err := r.signUpAnswering(t, "grace@example.com", termsAccepted)

	require.NoError(t, err)
	assert.Equal(t, []string{termsAcceptedRecord, noticeShownRecord}, r.decided(t, "grace@example.com"))
}

func TestAnUnknownItemOrAStaleVersionInTheAnswersIsRefused(t *testing.T) {
	r := newRig(t, 5)
	r.withItems(t)
	cases := map[string]consentrecords.Answer{
		"unknown item":  answer("no-such-item", fixtures.ServiceTermsCurrentLabel, true),
		"stale version": answer(fixtures.ServiceTermsKey, fixtures.ServiceTermsStaleLabel, true),
	}
	for name, wrong := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := r.signUpAnswering(t, "grace@example.com", wrong)

			assert.Equal(t, []string{"signupItems"}, refusedFields(t, err))
		})
	}
}

// With no item in effect (the fixture items are archived) a Sign-up asks and records nothing.
func TestWithNoItemsInEffectASignUpAsksAndRecordsNothing(t *testing.T) {
	r := newRig(t, 5)

	res, err := r.signUpAnswering(t, "grace@example.com")

	require.NoError(t, err)
	assert.Equal(t, signup.Accepted, res.Outcome)
	assert.Empty(t, r.decided(t, "grace@example.com"))
}

// The answers of a Sign-up of a Verified address are never written onto its User: whoever sent
// them has not proven the address.
func TestASignUpOfAVerifiedAddressRecordsNothingOnItsUser(t *testing.T) {
	r := newRig(t, 5)
	r.withItems(t)

	res, err := r.signUpAnswering(t, fixtures.OwnerJohnEmail, termsAccepted, newsUnticked)

	require.NoError(t, err)
	assert.Equal(t, signup.Accepted, res.Outcome)
	assert.Empty(t, r.decided(t, fixtures.OwnerJohnEmail))
}

// A repeated Sign-up of an Unverified address adds its answers, which become the current ones.
func TestARepeatedSignUpOfAnUnverifiedAddressRecordsItsAnswers(t *testing.T) {
	r := newRig(t, 5)
	r.withItems(t)
	_, err := r.signUpAnswering(t, fixtures.FreshUnverifiedUserEmail, termsAccepted, newsTicked)
	require.NoError(t, err)

	_, err = r.signUpAnswering(t, fixtures.FreshUnverifiedUserEmail, termsAccepted, newsUnticked)

	require.NoError(t, err)
	decided := r.decided(t, fixtures.FreshUnverifiedUserEmail)
	assert.Equal(t, []string{termsAcceptedRecord, newsDeclinedRecord, noticeShownRecord}, decided)
	assert.NotContains(t, decided, newsAcceptedRecord)
}

func TestWithVerificationOffTheAnswersAreRecordedOnTheVerifiedUser(t *testing.T) {
	r := newRig(t, 5, verificationOff)
	r.withItems(t)

	_, err := r.signUpAnswering(t, "grace@example.com", termsAccepted, newsTicked)

	require.NoError(t, err)
	assert.Equal(t, []string{termsAcceptedRecord, newsAcceptedRecord, noticeShownRecord}, r.decided(t, "grace@example.com"))
}
