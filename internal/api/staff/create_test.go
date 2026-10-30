package staff_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/staffmembers"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestACreatedStaffEnrolsAtFirstLoginWithItsOneTimePassword(t *testing.T) {
	h := newHarness(t)

	password, err := h.env.StaffMembers.Create(t.Context(), "  New.Staff@Sphericon.test ")
	require.NoError(t, err)
	require.NotEmpty(t, password)

	res := h.challenge(t, "new.staff@sphericon.test", password)
	assert.Equal(t, staffapi.StaffLoginOutcomeEnrolment, res.Outcome)
	require.True(t, res.Enrolment.Set)
	done, ok := h.secondStep(t, res.Challenge, h.code(t, res.Enrolment.Value.Secret)).(*staffapi.StaffResourceHeaders)
	require.True(t, ok)
	assert.EqualValues(t, "new.staff@sphericon.test", done.Response.Email)
}

func TestCreatingAnStaffWithATakenEmailIsRefused(t *testing.T) {
	h := newHarness(t)

	_, err := h.env.StaffMembers.Create(t.Context(), fixtures.StaffEnrolledEmail)
	require.ErrorIs(t, err, staffmembers.ErrDuplicate)

	_, err = h.env.StaffMembers.Create(t.Context(), " "+fixtures.StaffFreshEmail+" ")
	require.ErrorIs(t, err, staffmembers.ErrDuplicate, "the email is compared normalized")
}

func TestWithoutAStaffSecretNoStaffIsCreated(t *testing.T) {
	h := newHarness(t, testhelper.WithoutStaffSecret())

	_, err := h.env.StaffMembers.Create(t.Context(), "new.staff@sphericon.test")

	require.ErrorIs(t, err, staffmembers.ErrSignInOff)
}
