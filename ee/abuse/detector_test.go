package abuse_test

import (
	"crypto/ed25519"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/abuse"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func licenseFor(t *testing.T, features ...licensekey.Feature) *licensekey.License {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	key, err := licensekey.Issue(priv, nil, features...)
	require.NoError(t, err)
	lic, err := licensekey.Parse(key, pub, time.Now())
	require.NoError(t, err)
	return lic
}

const support = "support@sphericon.test"

func detector(t *testing.T, env *testhelper.TestEnv) *abuse.Detector {
	t.Helper()
	return abuse.New(env.DB, licenseFor(t, licensekey.FeatureStaff), env.Bus, env.SystemMail,
		abuse.Config{Limits: limits, SupportEmail: support})
}

// The traffic the tests judge is in fixtures/events.yml, one scenario per day offset: a test runs
// the detector at `at(days)` and sees only that scenario in the trailing 24 hours. The volumes are
// small, so the detector judges with a volume floor of 20 sent instead of the default 1000.
const (
	overDay      = 3  // Vandelay: 15% complaints, over the freeze limit; Kramerica, exempt: 50%
	floorDay     = 6  // Vandelay: 19 sent, all bounced hard, under the floor
	multiDay     = 33 // Vandelay: a clean mail domain and a news domain over the freeze limit
	warnDay      = 36 // Vandelay: 7.5% complaints, in the warning tier
	warnAgainDay = 34 // the same traffic once more, 48 hours later
)

var limits = abuse.Limits{VolumeFloor: 20, WarnComplaintBP: 500, WarnBounceBP: 1000, FreezeComplaintBP: 1000, FreezeBounceBP: 2000}

func at(days int) time.Time { return time.Now().Add(-time.Duration(days) * 24 * time.Hour) }

func TestAWorkspaceOverTheFreezeLimitIsFrozenAndItsOwnersTold(t *testing.T) {
	env := testhelper.Setup(t)

	summary, err := detector(t, env).Run(t.Context(), at(overDay))

	require.NoError(t, err)
	assert.Equal(t, []int64{fixtures.VandelayID}, summary.Frozen)
	ws := env.DB.Workspace.GetX(t.Context(), fixtures.VandelayID)
	require.NotNil(t, ws.SuspendedAt)
	assert.Equal(t, workspace.SuspendedByKindSystem, *ws.SuspendedByKind)
	assert.Contains(t, *ws.SuspensionReason, "complaint")
	assert.Contains(t, *ws.SuspensionReason, "mail.vandelay.test")
	assert.ElementsMatch(t, []string{fixtures.VandelayOwnerArtEmail, fixtures.VandelayOwnerCosmoEmail}, recipients(env), "both owners are told")
	for _, m := range env.SystemMail.Messages() {
		assert.Contains(t, m.Text, support)
	}
}

func suspended(t *testing.T, env *testhelper.TestEnv, ws int64) bool {
	t.Helper()
	return env.DB.Workspace.GetX(t.Context(), ws).SuspendedAt != nil
}

func recipients(env *testhelper.TestEnv) []string {
	var to []string
	for _, m := range env.SystemMail.Messages() {
		to = append(to, m.To)
	}
	return to
}

func TestNothingRunsWithoutTheStaffLicense(t *testing.T) {
	env := testhelper.Setup(t)

	for _, lic := range []*licensekey.License{nil, licenseFor(t), licenseFor(t, licensekey.FeatureRetention)} {
		d := abuse.New(env.DB, lic, env.Bus, env.SystemMail, abuse.Config{Limits: limits})
		summary, err := d.Run(t.Context(), at(overDay))
		require.NoError(t, err)
		assert.Equal(t, abuse.Summary{}, summary)
	}
	assert.False(t, suspended(t, env, fixtures.VandelayID))
	assert.Empty(t, env.SystemMail.Messages())
}

func TestBelowTheVolumeFloorNothingHappensEvenAtAHundredPercent(t *testing.T) {
	env := testhelper.Setup(t)

	summary, err := detector(t, env).Run(t.Context(), at(floorDay)) // every send bounced hard

	require.NoError(t, err)
	assert.Equal(t, abuse.Summary{}, summary)
	assert.False(t, suspended(t, env, fixtures.VandelayID))
	assert.Empty(t, env.SystemMail.Messages())
}

// Each limit at and just under its boundary, with the floor's worth of sends: a rate
// exactly at the limit reaches the tier, one event fewer does not. The scenarios are in
// fixtures/events.yml, one per day.
func TestEachLimitAtAndJustUnderItsBoundary(t *testing.T) {
	cases := []struct {
		name       string
		day        int
		wantFrozen bool
		wantWarned bool
	}{
		{"complaints at the freeze limit (10%)", 9, true, false},
		{"complaints just under the freeze limit", 12, false, true},
		{"complaints at the warning limit (5%)", 15, false, true},
		{"complaints just under the warning limit", 18, false, false},
		{"bounces at the freeze limit (20%)", 21, true, false},
		{"bounces just under the freeze limit", 24, false, true},
		{"bounces at the warning limit (10%)", 27, false, true},
		{"bounces just under the warning limit", 30, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := testhelper.Setup(t)

			summary, err := detector(t, env).Run(t.Context(), at(c.day))

			require.NoError(t, err)
			assert.Equal(t, c.wantFrozen, suspended(t, env, fixtures.VandelayID))
			assert.Equal(t, c.wantWarned, slices.Contains(summary.Warned, fixtures.VandelayID))
			assert.Equal(t, c.wantFrozen, slices.Contains(summary.Frozen, fixtures.VandelayID))
		})
	}
}

func TestAWarningMailsEveryOwnerOnceAndFreezesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	d := detector(t, env)

	first, err := d.Run(t.Context(), at(warnDay))
	require.NoError(t, err)
	second, err := d.Run(t.Context(), at(warnDay).Add(time.Minute))
	require.NoError(t, err)

	assert.Equal(t, []int64{fixtures.VandelayID}, first.Warned)
	assert.Empty(t, second.Warned, "one email per episode, not one per hour")
	assert.False(t, suspended(t, env, fixtures.VandelayID))
	assert.ElementsMatch(t, []string{fixtures.VandelayOwnerArtEmail, fixtures.VandelayOwnerCosmoEmail}, recipients(env), "every owner")
	assert.Empty(t, env.CustomerMail.Messages(), "the notice never goes through the Workspace's own Integration")
	assert.Contains(t, env.SystemMail.Messages()[0].Text, "mail.vandelay.test")
}

func TestARecoveredWorkspaceStartsANewEpisodeWhenItReEntersTheTier(t *testing.T) {
	env := testhelper.Setup(t)
	d := detector(t, env)
	_, err := d.Run(t.Context(), at(warnDay))
	require.NoError(t, err)
	require.Len(t, env.SystemMail.Messages(), 2)

	recovered, err := d.Run(t.Context(), at(warnDay-1)) // the old traffic has left the 24-hour window
	require.NoError(t, err)
	assert.Empty(t, recovered.Warned)

	again, err := d.Run(t.Context(), at(warnAgainDay)) // the same traffic, once more
	require.NoError(t, err)
	assert.Equal(t, []int64{fixtures.VandelayID}, again.Warned)
	assert.Len(t, env.SystemMail.Messages(), 4)
}

func TestAWarningWhoseMailFailedIsRetriedNextPass(t *testing.T) {
	env := testhelper.Setup(t)
	d := detector(t, env)
	env.SystemMail.SetErr(assert.AnError)

	_, err := d.Run(t.Context(), at(warnDay))
	require.Error(t, err)
	env.SystemMail.SetErr(nil)
	retried, err := d.Run(t.Context(), at(warnDay))

	require.NoError(t, err)
	assert.Equal(t, []int64{fixtures.VandelayID}, retried.Warned)
}

func TestAnyOneOfSeveralDomainsFreezesTheWholeWorkspace(t *testing.T) {
	env := testhelper.Setup(t)

	summary, err := detector(t, env).Run(t.Context(), at(multiDay))

	require.NoError(t, err)
	assert.Equal(t, []int64{fixtures.VandelayID}, summary.Frozen)
	assert.Contains(t, *env.DB.Workspace.GetX(t.Context(), fixtures.VandelayID).SuspensionReason, "news.vandelay.test")
}

func TestAFreezeIsAuditedAsTheSystemAndSendsNoWarning(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := detector(t, env).Run(t.Context(), at(overDay))
	require.NoError(t, err)
	env.DeliverToEE(t)

	entries := env.DB.Scoped(fixtures.VandelayID).AuditEntry().Query().
		Where(auditentry.Action(events.ActionWorkspaceSuspend)).AllX(t.Context())
	require.Len(t, entries, 1)
	assert.Equal(t, string(events.ActorSystem), string(entries[0].ActorKind))
	for _, m := range env.SystemMail.Messages() {
		assert.Contains(t, m.Subject, "suspended", "a direct freeze mails the freeze notice only")
	}
}

func TestAFailedFreezeNoticeLeavesTheFreezeInPlace(t *testing.T) {
	env := testhelper.Setup(t)
	env.SystemMail.SetErr(assert.AnError)

	summary, err := detector(t, env).Run(t.Context(), at(overDay))

	require.NoError(t, err, "logged, not retried: a suspended Workspace is skipped from now on")
	assert.Equal(t, []int64{fixtures.VandelayID}, summary.Frozen)
	assert.True(t, suspended(t, env, fixtures.VandelayID))
}

func TestASuspendedWorkspaceIsSkippedAndKeepsItsFirstActorAndReason(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := suspension.SuspendWorkspace(t.Context(), env.Bus, fixtures.VandelayID, suspension.StaffMember("op-7"), "chargeback")
	require.NoError(t, err)

	summary, err := detector(t, env).Run(t.Context(), at(overDay))

	require.NoError(t, err)
	assert.Equal(t, abuse.Summary{}, summary)
	ws := env.DB.Workspace.GetX(t.Context(), fixtures.VandelayID)
	assert.Equal(t, workspace.SuspendedByKindStaff, *ws.SuspendedByKind)
	assert.Equal(t, "chargeback", *ws.SuspensionReason)
	assert.Empty(t, env.SystemMail.Messages())
}

func TestTheDetectorNeverLiftsASuspension(t *testing.T) {
	env := testhelper.Setup(t)
	d := detector(t, env)
	_, err := d.Run(t.Context(), at(overDay))
	require.NoError(t, err)

	_, err = d.Run(t.Context(), at(overDay-2)) // clean by then

	require.NoError(t, err)
	assert.True(t, suspended(t, env, fixtures.VandelayID))
	assert.True(t, suspended(t, env, fixtures.HooliID), "a suspension by anyone else stands too")
}

func TestAnExemptWorkspaceIsNeitherWarnedNorFrozenButCanBeSuspendedByHand(t *testing.T) {
	env := testhelper.Setup(t)

	summary, err := detector(t, env).Run(t.Context(), at(overDay)) // Kramerica is at 50% complaints

	require.NoError(t, err)
	assert.NotContains(t, summary.Frozen, fixtures.KramericaID)
	assert.NotContains(t, summary.Warned, fixtures.KramericaID)
	assert.False(t, suspended(t, env, fixtures.KramericaID))
	assert.NotContains(t, recipients(env), fixtures.KramericaOwnerNewmanEmail)
	changed, err := suspension.SuspendWorkspace(t.Context(), env.Bus, fixtures.KramericaID, suspension.CLI, "verified abuse")
	require.NoError(t, err)
	assert.True(t, changed, "an exemption is not a loophole for real abuse")
}

// The clearance scenario is Kramerica's traffic at day 0 in fixtures/events.yml: over the freeze
// limit an hour ago, 19 sent with 3 complaints an hour ahead and the 20th send two hours ahead.
// The test lifts Kramerica's exemption, which the detector would otherwise honour.
func TestAfterAnUnsuspendOnlyNewerEventsCountUntilTheFloorIsMetAgain(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Now()
	env.DB.Workspace.UpdateOneID(fixtures.KramericaID).SetAbuseExempt(false).ExecX(t.Context())
	d := detector(t, env)
	_, err := d.Run(t.Context(), now)
	require.NoError(t, err)
	require.True(t, suspended(t, env, fixtures.KramericaID))
	_, err = suspension.UnsuspendWorkspace(t.Context(), env.Bus, fixtures.KramericaID, suspension.StaffMember("1"))
	require.NoError(t, err)

	immediately, err := d.Run(t.Context(), now.Add(time.Minute))
	require.NoError(t, err)
	assert.Empty(t, immediately.Frozen, "the same old complaints do not freeze a cleared sender again")

	underFloor, err := d.Run(t.Context(), now.Add(90*time.Minute))
	require.NoError(t, err)
	assert.Empty(t, underFloor.Frozen, "under the floor since the clearance")

	refrozen, err := d.Run(t.Context(), now.Add(3*time.Hour)) // the 20th send is in
	require.NoError(t, err)
	assert.Equal(t, []int64{fixtures.KramericaID}, refrozen.Frozen)
}

func TestUnsuspendingRecordsTheClearance(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := suspension.UnsuspendWorkspace(t.Context(), env.Bus, fixtures.HooliID, suspension.StaffMember("1"))
	require.NoError(t, err)

	assert.NotNil(t, env.DB.Workspace.GetX(t.Context(), fixtures.HooliID).ClearedAt)
}

func TestTheDashboardWarningIsTheDetectorsOwnComputation(t *testing.T) {
	env := testhelper.Setup(t)
	d := detector(t, env)
	ws := env.DB.Workspace.GetX(t.Context(), fixtures.VandelayID)

	breach, err := d.Warning(t.Context(), env.DB.Scoped(fixtures.VandelayID), ws, at(warnDay))

	require.NoError(t, err)
	assert.Equal(t, abuse.TierWarn, breach.Tier)
	assert.Equal(t, abuse.Complaint, breach.Metric)
	assert.Equal(t, "mail.vandelay.test", breach.Domain)
}

func TestAWarnedWorkspaceThatIsThenSuspendedByHandEndsItsEpisode(t *testing.T) {
	env := testhelper.Setup(t)
	d := detector(t, env)
	_, err := d.Run(t.Context(), at(warnDay))
	require.NoError(t, err)
	require.NotNil(t, env.DB.Workspace.GetX(t.Context(), fixtures.VandelayID).AbuseWarningNotifiedAt)

	_, err = suspension.SuspendWorkspace(t.Context(), env.Bus, fixtures.VandelayID, suspension.CLI, "manual")
	require.NoError(t, err)
	_, err = d.Run(t.Context(), at(warnDay).Add(time.Minute))

	require.NoError(t, err)
	assert.Nil(t, env.DB.Workspace.GetX(t.Context(), fixtures.VandelayID).AbuseWarningNotifiedAt)
}

func TestAWorkspaceInItsDeletionWindowIsNotJudged(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.VandelayID).SetDiscardedAt(at(overDay)).ExecX(t.Context())

	summary, err := detector(t, env).Run(t.Context(), at(overDay))

	require.NoError(t, err)
	assert.Equal(t, abuse.Summary{}, summary)
	assert.False(t, suspended(t, env, fixtures.VandelayID))
	assert.Empty(t, env.SystemMail.Messages())
}
