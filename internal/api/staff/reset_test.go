package staff_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/staffmembers"
)

func TestAResetTOTPForcesReenrolmentAtNextLogin(t *testing.T) {
	h := newHarness(t)

	require.NoError(t, h.env.StaffMembers.ResetTOTP(t.Context(), " "+fixtures.StaffEnrolledEmail+" "))

	res := h.challenge(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword)
	assert.Equal(t, staffapi.StaffLoginOutcomeEnrolment, res.Outcome, "the password alone never gives a session")
	require.True(t, res.Enrolment.Set)
	assert.NotEqual(t, fixtures.StaffEnrolledTotpSecret, res.Enrolment.Value.Secret, "a fresh secret")
	done, ok := h.secondStep(t, res.Challenge, h.code(t, res.Enrolment.Value.Secret)).(*staffapi.StaffResourceHeaders)
	require.True(t, ok)
	assert.EqualValues(t, fixtures.StaffEnrolledEmail, done.Response.Email)
}

func TestAResetTOTPRefusesTheOldAuthenticator(t *testing.T) {
	h := newHarness(t)
	old := h.challenge(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword).Challenge

	require.NoError(t, h.env.StaffMembers.ResetTOTP(t.Context(), fixtures.StaffEnrolledEmail))

	assert.Equal(t, http.StatusUnauthorized, h.rawSecondStep(t, old, h.code(t, fixtures.StaffEnrolledTotpSecret)).Code,
		"a challenge minted before the reset is dead")
	res := h.challenge(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword)
	requireProblem(t, h.secondStep(t, res.Challenge, h.code(t, fixtures.StaffEnrolledTotpSecret)), http.StatusUnauthorized)
}

func TestAResetTOTPEndsTheStaffMembersSessions(t *testing.T) {
	h := newHarness(t)
	cookie := h.session(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword, fixtures.StaffEnrolledTotpSecret)

	require.NoError(t, h.env.StaffMembers.ResetTOTP(t.Context(), fixtures.StaffEnrolledEmail))

	requireProblem(t, h.me(t, cookie.Value), http.StatusUnauthorized)
}

func TestResettingAnUnknownStaffIsRefused(t *testing.T) {
	h := newHarness(t)

	require.ErrorIs(t, h.env.StaffMembers.ResetTOTP(t.Context(), "nobody@sphericon.test"), staffmembers.ErrNotFound)
}

// A lost TOTP is recoverable only through the CLI: no web route resets it.
func TestNoEndpointResetsATOTP(t *testing.T) {
	h := newHarness(t)
	cookie := h.session(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword, fixtures.StaffEnrolledTotpSecret)

	for _, path := range []string{"/staff/auth/reset-totp", "/staff/auth/reset", "/staff/totp/reset", "/staff/me/totp"} {
		rec := post(t, h.env.Server, path, `{"email":"`+fixtures.StaffEnrolledEmail+`"}`)
		assert.Contains(t, []int{http.StatusNotFound, http.StatusMethodNotAllowed}, rec.Code, path)
		assert.Contains(t, []int{http.StatusNotFound, http.StatusMethodNotAllowed}, get(t, h.env.Server, path, cookie).Code, path)
	}
}
