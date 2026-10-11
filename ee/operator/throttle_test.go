package operator_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mokevnin/sphericon/config"
	"github.com/mokevnin/sphericon/ent/authattempt"
	operatorapi "github.com/mokevnin/sphericon/gen/operator"
	"github.com/mokevnin/sphericon/internal/fixtures"
	"github.com/mokevnin/sphericon/internal/testhelper"
)

const loginFailures = 5

func throttledHarness(t *testing.T, ipLimit int) *harness {
	t.Helper()
	return newHarness(t, testhelper.WithRateLimits(config.RateLimits{LoginFailures: loginFailures, LoginIP: ipLimit}))
}

func (h *harness) rawPasswordStep(t *testing.T, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	return post(t, h.env.Server, "/operator/auth/login", `{"email":"`+email+`","password":"`+password+`"}`)
}

func (h *harness) rawSecondStep(t *testing.T, challenge, code string) *httptest.ResponseRecorder {
	t.Helper()
	return post(t, h.env.Server, "/operator/auth/second-factor", `{"challenge":"`+challenge+`","code":"`+code+`"}`)
}

func (h *harness) failPasswords(t *testing.T, email string, times int) {
	t.Helper()
	for i := range times {
		require.Equal(t, http.StatusUnauthorized, h.rawPasswordStep(t, email, "wrong-password").Code, "failure %d", i+1)
	}
}

func requireThrottled(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
	assert.Equal(t, "1", rec.Header().Get("Retry-After"))
	assert.Equal(t, "0", rec.Header().Get("X-RateLimit-Remaining"))
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	assert.Empty(t, rec.Header().Get("Set-Cookie"))
}

func TestRepeatedWrongPasswordsAreRefusedEvenForTheRightPassword(t *testing.T) {
	h := throttledHarness(t, 0)
	h.failPasswords(t, fixtures.OperatorEnrolledEmail, loginFailures)

	requireThrottled(t, h.rawPasswordStep(t, fixtures.OperatorEnrolledEmail, "wrong-password"))
	requireThrottled(t, h.rawPasswordStep(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword))
}

func TestThePasswordStepWorksAgainOnceTheDelayHasPassed(t *testing.T) {
	h := throttledHarness(t, 0)
	h.failPasswords(t, fixtures.OperatorEnrolledEmail, loginFailures)
	requireThrottled(t, h.rawPasswordStep(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword))

	h.clock.t = h.clock.t.Add(2 * time.Second)

	assert.Equal(t, operatorapi.OperatorLoginOutcomeChallenge, h.challenge(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword).Outcome)
}

func TestUnknownEmailsAreThrottledLikeKnownOnes(t *testing.T) {
	h := throttledHarness(t, 0)
	h.failPasswords(t, "nobody@sphericon.test", loginFailures)

	requireThrottled(t, h.rawPasswordStep(t, "nobody@sphericon.test", "wrong-password"))
	n, err := h.env.DB.AuthAttempt.Query().Where(authattempt.Email("nobody@sphericon.test")).Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the failures of an unknown address leave a row, so the table does not tell accounts apart")
}

func TestWrongTOTPCodesAreRefusedEvenForTheRightCode(t *testing.T) {
	h := throttledHarness(t, 0)
	challenge := h.challenge(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword).Challenge
	for i := range loginFailures {
		require.Equal(t, http.StatusUnauthorized, h.rawSecondStep(t, challenge, "000000").Code, "failure %d", i+1)
	}

	requireThrottled(t, h.rawSecondStep(t, challenge, h.code(t, fixtures.OperatorEnrolledTotpSecret)))
	requireThrottled(t, h.rawPasswordStep(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword))
}

// Knowing the password must not buy a fresh round of code guesses.
func TestThePasswordStepDoesNotResetTheCodeFailures(t *testing.T) {
	h := throttledHarness(t, 0)
	for i := range loginFailures {
		challenge := h.challenge(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword).Challenge
		require.Equal(t, http.StatusUnauthorized, h.rawSecondStep(t, challenge, "000000").Code, "failure %d", i+1)
	}

	requireThrottled(t, h.rawPasswordStep(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword))
}

func TestASessionResetsTheCounter(t *testing.T) {
	h := throttledHarness(t, 0)
	challenge := h.challenge(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword).Challenge
	for range loginFailures - 1 {
		require.Equal(t, http.StatusUnauthorized, h.rawSecondStep(t, challenge, "000000").Code)
	}

	rec := h.rawSecondStep(t, challenge, h.code(t, fixtures.OperatorEnrolledTotpSecret))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	n, err := h.env.DB.AuthAttempt.Query().Where(authattempt.Email(fixtures.OperatorEnrolledEmail)).Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, n)
}

// An Operator who is also a customer has two identities: the Operator's failures
// count under their own kind and never throttle the User of the same address.
func TestOperatorFailuresNeverCountAgainstTheUserOfTheSameAddress(t *testing.T) {
	h := throttledHarness(t, 0)
	h.failPasswords(t, fixtures.OperatorAlsoCustomerEmail, loginFailures)

	rows, err := h.env.DB.AuthAttempt.Query().Where(authattempt.Email(fixtures.OperatorAlsoCustomerEmail)).All(t.Context())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, authattempt.KindOperatorLogin, rows[0].Kind)

	rec := post(t, h.env.Server, "/site/auth/login",
		`{"email":"`+fixtures.OwnerJohnEmail+`","password":"`+fixtures.OwnerJohnPassword+`"}`)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestTheOperatorLoginSharesThePerIPLoginCap(t *testing.T) {
	h := throttledHarness(t, 2)
	for range 2 {
		require.Equal(t, http.StatusUnauthorized, h.rawPasswordStep(t, "nobody@sphericon.test", "x").Code)
	}

	rec := h.rawPasswordStep(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

func TestWithoutLimitsNothingIsThrottled(t *testing.T) {
	h := newHarness(t)
	h.failPasswords(t, fixtures.OperatorEnrolledEmail, loginFailures*2)

	assert.Equal(t, operatorapi.OperatorLoginOutcomeChallenge, h.challenge(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword).Outcome)
}
