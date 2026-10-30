package accounts_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Umbrella requires a Second factor since 2026-03-01: Rita's grace ends 2026-03-08, Sam has a
// Second factor, and Nina joined after the requirement, so her grace is her own.
var (
	inRitasGrace  = time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC)
	afterRitasEnd = time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
)

// admission builds the Accounts of the test's database and loads userID's Membership of the
// Workspace publicID names, as a surface would before admitting it.
func admission(t *testing.T, env *testhelper.TestEnv, userID int64, publicID string) (*accounts.Accounts, *ent.Membership) {
	t.Helper()
	acc := accounts.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords)
	_, m, err := acc.Scope(t.Context(), userID, publicID)
	require.NoError(t, err)
	return acc, m
}

func TestAMemberWithNothingPendingIsAdmitted(t *testing.T) {
	env := testhelper.Setup(t)
	acc, m := admission(t, env, fixtures.MemberMaryID, fixtures.AcmePublicID)

	err := acc.Admit(t.Context(), accounts.Admission{UserID: fixtures.MemberMaryID, Membership: m, At: afterRitasEnd})

	require.NoError(t, err)
}

func TestAUserWithARequiredAcceptancePendingIsRefusedInAndOutOfAWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	acc, m := admission(t, env, fixtures.MemberMaryID, fixtures.AcmePublicID)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	inWorkspace := acc.Admit(t.Context(), accounts.Admission{UserID: fixtures.MemberMaryID, Membership: m})
	outside := acc.Admit(t.Context(), accounts.Admission{UserID: fixtures.MemberMaryID})
	letThrough := acc.Admit(t.Context(), accounts.Admission{UserID: fixtures.MemberMaryID, Membership: m, WhileAcceptancePending: true})

	require.ErrorIs(t, inWorkspace, accounts.ErrAcceptancePending)
	require.ErrorIs(t, outside, accounts.ErrAcceptancePending)
	require.NoError(t, letThrough)
}

func TestTheTwoFactorRequirementWithholdsAMemberOnceTheGraceEnds(t *testing.T) {
	env := testhelper.Setup(t)
	acc, m := admission(t, env, fixtures.UmbrellaOwnerRitaID, fixtures.UmbrellaPublicID)
	admit := func(r accounts.Admission) error {
		r.UserID, r.Membership = fixtures.UmbrellaOwnerRitaID, m
		return acc.Admit(t.Context(), r)
	}

	require.NoError(t, admit(accounts.Admission{At: inRitasGrace}), "within the grace")
	require.ErrorIs(t, admit(accounts.Admission{At: afterRitasEnd}), accounts.ErrSecondFactorRequired, "after it")
	require.NoError(t, admit(accounts.Admission{At: afterRitasEnd, StrongSession: true}), "a strong session meets the requirement")
	require.NoError(t, admit(accounts.Admission{At: afterRitasEnd, WhileWithheld: true}), "an operation that lets a withheld User through")
}

func TestTheTwoFactorRequirementDoesNotWithholdOutsideItsWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	acc := accounts.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords)

	err := acc.Admit(t.Context(), accounts.Admission{UserID: fixtures.UmbrellaOwnerRitaID, At: afterRitasEnd})

	require.NoError(t, err)
}

func TestAMemberWithASecondFactorIsNeverWithheld(t *testing.T) {
	env := testhelper.Setup(t)
	acc, m := admission(t, env, fixtures.SecondFactorSamID, fixtures.UmbrellaPublicID)

	err := acc.Admit(t.Context(), accounts.Admission{UserID: fixtures.SecondFactorSamID, Membership: m, At: afterRitasEnd})

	require.NoError(t, err)
}

func TestALaterMembershipHasAGraceOfItsOwn(t *testing.T) {
	env := testhelper.Setup(t)
	_, rita := admission(t, env, fixtures.UmbrellaOwnerRitaID, fixtures.UmbrellaPublicID)
	_, nina := admission(t, env, fixtures.UmbrellaMemberNinaID, fixtures.UmbrellaPublicID)

	ritasEnd, ritaHasOne := accounts.SecondFactorDeadline(rita)
	ninasEnd, ninaHasOne := accounts.SecondFactorDeadline(nina)

	require.True(t, ritaHasOne)
	require.True(t, ninaHasOne)
	assert.Equal(t, time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC), ritasEnd.UTC())
	assert.Equal(t, nina.CreatedAt.Add(accounts.GracePeriod), ninasEnd)
}

func TestAUserBothGatedAndWithheldIsRefusedForTheAcceptanceFirst(t *testing.T) {
	env := testhelper.Setup(t)
	acc, m := admission(t, env, fixtures.UmbrellaOwnerRitaID, fixtures.UmbrellaPublicID)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	both := acc.Admit(t.Context(), accounts.Admission{UserID: fixtures.UmbrellaOwnerRitaID, Membership: m, At: afterRitasEnd})
	acceptanceLifted := acc.Admit(t.Context(), accounts.Admission{UserID: fixtures.UmbrellaOwnerRitaID, Membership: m, At: afterRitasEnd, WhileAcceptancePending: true})

	require.ErrorIs(t, both, accounts.ErrAcceptancePending)
	require.ErrorIs(t, acceptanceLifted, accounts.ErrSecondFactorRequired)
}
