package secondfactors_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Umbrella requires a Second factor since 2026-03-01: Rita's grace ends 2026-03-08, Sam has a
// Second factor, and Nina joined after the requirement, so her grace is her own.
var (
	inRitasGrace  = time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC)
	afterRitasEnd = time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
)

// newModule builds the Module of the test's database.
func newModule(env *testhelper.TestEnv) *secondfactors.Module {
	acceptance := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, time.Now)
	return secondfactors.New(env.DB, env.Bus, env.Cipher, acceptance, time.Now)
}

// admission builds the Module of the test's database and loads userID's Membership of the
// Workspace publicID names, as the admission does before it asks the requirement.
func admission(t *testing.T, env *testhelper.TestEnv, userID int64, publicID string) (*secondfactors.Module, *ent.Membership) {
	t.Helper()
	_, m, err := memberships.New(env.DB, env.Bus).Scope(t.Context(), userID, publicID)
	require.NoError(t, err)
	return newModule(env), m
}

// check asks the requirement of the Workspace of m about its member at at.
func check(t *testing.T, factors *secondfactors.Module, m *ent.Membership, at time.Time) error {
	t.Helper()
	return factors.CheckRequirement(t.Context(), m.UserID, m.Edges.Workspace.SecondFactorRequiredAt, m.CreatedAt, at)
}

func TestAWorkspaceWithNoRequirementWithholdsNoOne(t *testing.T) {
	env := testhelper.Setup(t)
	factors, m := admission(t, env, fixtures.MemberMaryID, fixtures.AcmePublicID)

	require.NoError(t, check(t, factors, m, afterRitasEnd))
}

func TestTheTwoFactorRequirementWithholdsAMemberOnceTheGraceEnds(t *testing.T) {
	env := testhelper.Setup(t)
	factors, m := admission(t, env, fixtures.UmbrellaOwnerRitaID, fixtures.UmbrellaPublicID)

	require.NoError(t, check(t, factors, m, inRitasGrace), "within the grace")
	require.ErrorIs(t, check(t, factors, m, afterRitasEnd), secondfactors.ErrSecondFactorRequired, "after it")
}

func TestAMemberWithASecondFactorIsNeverWithheld(t *testing.T) {
	env := testhelper.Setup(t)
	factors, m := admission(t, env, fixtures.SecondFactorSamID, fixtures.UmbrellaPublicID)

	require.NoError(t, check(t, factors, m, afterRitasEnd))
}

func TestALaterMembershipHasAGraceOfItsOwn(t *testing.T) {
	env := testhelper.Setup(t)
	acc, rita := admission(t, env, fixtures.UmbrellaOwnerRitaID, fixtures.UmbrellaPublicID)
	_, nina := admission(t, env, fixtures.UmbrellaMemberNinaID, fixtures.UmbrellaPublicID)

	ritasEnd, ritaHasOne, err := acc.Deadline(t.Context(), rita)
	require.NoError(t, err)
	ninasEnd, ninaHasOne, err := acc.Deadline(t.Context(), nina)
	require.NoError(t, err)

	require.True(t, ritaHasOne)
	require.True(t, ninaHasOne)
	assert.Equal(t, time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC), ritasEnd.UTC())
	assert.Equal(t, nina.CreatedAt.Add(secondfactors.GracePeriod), ninasEnd)
}

func TestAMemberWithASecondFactorHasNoDeadline(t *testing.T) {
	env := testhelper.Setup(t)
	acc, sam := admission(t, env, fixtures.SecondFactorSamID, fixtures.UmbrellaPublicID)
	_, rita := admission(t, env, fixtures.UmbrellaOwnerRitaID, fixtures.UmbrellaPublicID)

	deadlines, err := acc.Deadlines(t.Context(), []*ent.Membership{sam, rita})

	require.NoError(t, err)
	assert.Equal(t, map[int64]time.Time{rita.ID: time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)}, utc(deadlines))
}

func utc(ends map[int64]time.Time) map[int64]time.Time {
	out := make(map[int64]time.Time, len(ends))
	for id, end := range ends {
		out[id] = end.UTC()
	}
	return out
}
