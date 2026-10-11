package operator_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mokevnin/sphericon/ee/operator"
	operatorapi "github.com/mokevnin/sphericon/gen/operator"
	"github.com/mokevnin/sphericon/internal/fixtures"
	"github.com/mokevnin/sphericon/internal/testhelper"
)

func TestACreatedOperatorEnrolsAtFirstLoginWithItsOneTimePassword(t *testing.T) {
	h := newHarness(t)

	password, err := h.env.Operators.Create(t.Context(), "  New.Staff@Sphericon.test ")
	require.NoError(t, err)
	require.NotEmpty(t, password)

	res := h.challenge(t, "new.staff@sphericon.test", password)
	assert.Equal(t, operatorapi.OperatorLoginOutcomeEnrolment, res.Outcome)
	require.True(t, res.Enrolment.Set)
	done, ok := h.secondStep(t, res.Challenge, h.code(t, res.Enrolment.Value.Secret)).(*operatorapi.OperatorResourceHeaders)
	require.True(t, ok)
	assert.EqualValues(t, "new.staff@sphericon.test", done.Response.Email)
}

func TestCreatingAnOperatorWithATakenEmailIsRefused(t *testing.T) {
	h := newHarness(t)

	_, err := h.env.Operators.Create(t.Context(), fixtures.OperatorEnrolledEmail)
	require.ErrorIs(t, err, operator.ErrDuplicate)

	_, err = h.env.Operators.Create(t.Context(), " "+fixtures.OperatorFreshEmail+" ")
	require.ErrorIs(t, err, operator.ErrDuplicate, "the email is compared normalized")
}

func TestWithoutALicenseNoOperatorCanBeCreated(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())

	_, err := env.Operators.Create(t.Context(), "new.staff@sphericon.test")

	require.ErrorIs(t, err, operator.ErrNotLicensed)
}
