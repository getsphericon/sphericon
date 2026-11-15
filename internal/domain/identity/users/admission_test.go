package users_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestAUserWithNothingPendingPassesTheAcceptanceCheck(t *testing.T) {
	env := testhelper.Setup(t)
	acc := testhelper.Module[*users.Module](env)

	err := acc.CheckAcceptance(t.Context(), fixtures.MemberMaryID)

	require.NoError(t, err)
}

func TestAUserWithARequiredAcceptancePendingFailsTheAcceptanceCheck(t *testing.T) {
	env := testhelper.Setup(t)
	acc := testhelper.Module[*users.Module](env)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	err := acc.CheckAcceptance(t.Context(), fixtures.MemberMaryID)

	require.ErrorIs(t, err, users.ErrAcceptancePending)
}
