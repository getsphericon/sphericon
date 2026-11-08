package users_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestAUserWithNothingPendingPassesTheAcceptanceCheck(t *testing.T) {
	env := testhelper.Setup(t)
	acc := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, time.Now)

	err := acc.CheckAcceptance(t.Context(), fixtures.MemberMaryID)

	require.NoError(t, err)
}

func TestAUserWithARequiredAcceptancePendingFailsTheAcceptanceCheck(t *testing.T) {
	env := testhelper.Setup(t)
	acc := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, time.Now)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	err := acc.CheckAcceptance(t.Context(), fixtures.MemberMaryID)

	require.ErrorIs(t, err, users.ErrAcceptancePending)
}
