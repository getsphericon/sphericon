package operator_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mokevnin/sphericon/ee/operator"
	operatorapi "github.com/mokevnin/sphericon/gen/operator"
	"github.com/mokevnin/sphericon/internal/fixtures"
	"github.com/mokevnin/sphericon/internal/testhelper"
)

func TestAResetTOTPForcesReenrolmentAtNextLogin(t *testing.T) {
	h := newHarness(t)

	require.NoError(t, h.env.Operators.ResetTOTP(t.Context(), " "+fixtures.OperatorEnrolledEmail+" "))

	res := h.challenge(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword)
	assert.Equal(t, operatorapi.OperatorLoginOutcomeEnrolment, res.Outcome, "the password alone never gives a session")
	require.True(t, res.Enrolment.Set)
	assert.NotEqual(t, fixtures.OperatorEnrolledTotpSecret, res.Enrolment.Value.Secret, "a fresh secret")
	done, ok := h.secondStep(t, res.Challenge, h.code(t, res.Enrolment.Value.Secret)).(*operatorapi.OperatorResourceHeaders)
	require.True(t, ok)
	assert.EqualValues(t, fixtures.OperatorEnrolledEmail, done.Response.Email)
}

func TestAResetTOTPRefusesTheOldAuthenticator(t *testing.T) {
	h := newHarness(t)
	old := h.challenge(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword).Challenge

	require.NoError(t, h.env.Operators.ResetTOTP(t.Context(), fixtures.OperatorEnrolledEmail))

	assert.Equal(t, http.StatusUnauthorized, h.rawSecondStep(t, old, h.code(t, fixtures.OperatorEnrolledTotpSecret)).Code,
		"a challenge minted before the reset is dead")
	res := h.challenge(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword)
	requireProblem(t, h.secondStep(t, res.Challenge, h.code(t, fixtures.OperatorEnrolledTotpSecret)), http.StatusUnauthorized)
}

func TestAResetTOTPEndsTheOperatorsSessions(t *testing.T) {
	h := newHarness(t)
	cookie := h.session(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword, fixtures.OperatorEnrolledTotpSecret)

	require.NoError(t, h.env.Operators.ResetTOTP(t.Context(), fixtures.OperatorEnrolledEmail))

	requireProblem(t, h.me(t, cookie.Value), http.StatusUnauthorized)
}

func TestResettingAnUnknownOperatorIsRefused(t *testing.T) {
	h := newHarness(t)

	require.ErrorIs(t, h.env.Operators.ResetTOTP(t.Context(), "nobody@sphericon.test"), operator.ErrNotFound)
}

func TestWithoutALicenseNoTOTPCanBeReset(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())

	require.ErrorIs(t, env.Operators.ResetTOTP(t.Context(), fixtures.OperatorEnrolledEmail), operator.ErrNotLicensed)
}

// A lost TOTP is recoverable only through the CLI: no web route resets it.
func TestNoEndpointResetsATOTP(t *testing.T) {
	h := newHarness(t)
	cookie := h.session(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword, fixtures.OperatorEnrolledTotpSecret)

	for _, path := range []string{"/operator/auth/reset-totp", "/operator/auth/reset", "/operator/totp/reset", "/operator/me/totp"} {
		rec := post(t, h.env.Server, path, `{"email":"`+fixtures.OperatorEnrolledEmail+`"}`)
		assert.Contains(t, []int{http.StatusNotFound, http.StatusMethodNotAllowed}, rec.Code, path)
		assert.Contains(t, []int{http.StatusNotFound, http.StatusMethodNotAllowed}, get(t, h.env.Server, path, cookie).Code, path)
	}
}
