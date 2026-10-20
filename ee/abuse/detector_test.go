package abuse_test

import (
	"crypto/ed25519"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/abuse"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
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
		abuse.Config{Limits: abuse.DefaultLimits(), SupportEmail: support})
}

// lay puts a Sending domain's traffic into the Workspace, one hour before `now`.
func lay(t *testing.T, env *testhelper.TestEnv, ws int64, domain string, now time.Time, sent, hardBounces, complaints int) {
	t.Helper()
	s := env.DB.Scoped(ws)
	at := now.Add(-time.Hour)
	var creates = make([]*ent.EventScopedCreate, 0, sent+complaints)
	add := func(action string, props map[string]any, n int) {
		for i := range n {
			creates = append(creates, s.Event().Create().
				SetEmail(fmt.Sprintf("%s-%d@example.com", action, i)).
				SetAction(action).
				SetProperties(props).
				SetOccurredAt(at))
		}
	}
	add(events.NameEmailSent, map[string]any{"sendingDomain": domain}, sent)
	add(events.NameEmailBounced, map[string]any{"sendingDomain": domain, "bounceKind": events.BounceKindPermanent}, hardBounces)
	add(events.NameEmailComplained, map[string]any{"sendingDomain": domain}, complaints)
	require.NoError(t, s.Event().CreateBulk(creates...).Exec(t.Context()))
}

func TestAWorkspaceOverTheFreezeLimitIsFrozenAndItsOwnersTold(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Now()
	lay(t, env, fixtures.VandelayID, "mail.vandelay.test", now, 2000, 0, 10) // 0.5% complaints

	summary, err := detector(t, env).Run(t.Context(), now)

	require.NoError(t, err)
	assert.Equal(t, []int64{fixtures.VandelayID}, summary.Frozen)
	ws := env.DB.Workspace.GetX(t.Context(), fixtures.VandelayID)
	require.NotNil(t, ws.SuspendedAt)
	assert.Equal(t, workspace.SuspendedByKindSystem, *ws.SuspendedByKind)
	assert.Contains(t, *ws.SuspensionReason, "complaint")
	assert.Contains(t, *ws.SuspensionReason, "mail.vandelay.test")
	assert.Len(t, env.SystemMail.Messages(), 2, "both owners are told")
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
	now := time.Now()
	lay(t, env, fixtures.VandelayID, "mail.vandelay.test", now, 2000, 0, 100)

	for _, lic := range []*licensekey.License{nil, licenseFor(t), licenseFor(t, licensekey.FeatureRetention)} {
		d := abuse.New(env.DB, lic, env.Bus, env.SystemMail, abuse.Config{})
		summary, err := d.Run(t.Context(), now)
		require.NoError(t, err)
		assert.Equal(t, abuse.Summary{}, summary)
	}
	assert.False(t, suspended(t, env, fixtures.VandelayID))
	assert.Empty(t, env.SystemMail.Messages())
}

func TestBelowTheVolumeFloorNothingHappensEvenAtAHundredPercent(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Now()
	lay(t, env, fixtures.VandelayID, "mail.vandelay.test", now, 999, 999, 0) // every send bounced hard

	summary, err := detector(t, env).Run(t.Context(), now)

	require.NoError(t, err)
	assert.Equal(t, abuse.Summary{}, summary)
	assert.False(t, suspended(t, env, fixtures.VandelayID))
	assert.Empty(t, env.SystemMail.Messages())
}

// Each limit at and just under its boundary, with the floor's worth of sends: a rate
// exactly at the limit reaches the tier, one event fewer does not.
func TestEachLimitAtAndJustUnderItsBoundary(t *testing.T) {
	type outcome int
	const (
		quiet outcome = iota
		warned
		frozen
	)
	cases := []struct {
		name                          string
		sent, hardBounces, complaints int
		want                          outcome
	}{
		{"complaints at the freeze limit (0.3%)", 2000, 0, 6, frozen},
		{"complaints just under the freeze limit", 2000, 0, 5, warned},
		{"complaints at the warning limit (0.1%)", 2000, 0, 2, warned},
		{"complaints just under the warning limit", 2000, 0, 1, quiet},
		{"bounces at the freeze limit (10%)", 1000, 100, 0, frozen},
		{"bounces just under the freeze limit", 1000, 99, 0, warned},
		{"bounces at the warning limit (5%)", 1000, 50, 0, warned},
		{"bounces just under the warning limit", 1000, 49, 0, quiet},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			now := time.Now()
			lay(t, env, fixtures.VandelayID, "mail.vandelay.test", now, c.sent, c.hardBounces, c.complaints)

			summary, err := detector(t, env).Run(t.Context(), now)

			require.NoError(t, err)
			assert.Equal(t, c.want == frozen, suspended(t, env, fixtures.VandelayID))
			assert.Equal(t, c.want == warned, len(summary.Warned) == 1)
			assert.Equal(t, c.want == frozen, len(summary.Frozen) == 1)
		})
	}
}

func TestAWarningMailsEveryOwnerOnceAndFreezesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Now()
	lay(t, env, fixtures.VandelayID, "mail.vandelay.test", now, 2000, 0, 3) // 0.15%
	d := detector(t, env)

	first, err := d.Run(t.Context(), now)
	require.NoError(t, err)
	second, err := d.Run(t.Context(), now.Add(time.Minute))
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
	now := time.Now()
	lay(t, env, fixtures.VandelayID, "mail.vandelay.test", now, 2000, 0, 3)
	d := detector(t, env)
	_, err := d.Run(t.Context(), now)
	require.NoError(t, err)
	require.Len(t, env.SystemMail.Messages(), 2)

	later := now.Add(48 * time.Hour) // the old traffic has left the 24-hour window
	recovered, err := d.Run(t.Context(), later)
	require.NoError(t, err)
	assert.Empty(t, recovered.Warned)

	lay(t, env, fixtures.VandelayID, "mail.vandelay.test", later, 2000, 0, 3)
	again, err := d.Run(t.Context(), later)
	require.NoError(t, err)
	assert.Equal(t, []int64{fixtures.VandelayID}, again.Warned)
	assert.Len(t, env.SystemMail.Messages(), 4)
}

func TestAWarningWhoseMailFailedIsRetriedNextPass(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Now()
	lay(t, env, fixtures.VandelayID, "mail.vandelay.test", now, 2000, 0, 3)
	d := detector(t, env)
	env.SystemMail.SetErr(assert.AnError)

	_, err := d.Run(t.Context(), now)
	require.Error(t, err)
	env.SystemMail.SetErr(nil)
	retried, err := d.Run(t.Context(), now)

	require.NoError(t, err)
	assert.Equal(t, []int64{fixtures.VandelayID}, retried.Warned)
}

func TestAnyOneOfSeveralDomainsFreezesTheWholeWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Now()
	lay(t, env, fixtures.VandelayID, "mail.vandelay.test", now, 5000, 0, 0)
	lay(t, env, fixtures.VandelayID, "news.vandelay.test", now, 2000, 0, 10)

	summary, err := detector(t, env).Run(t.Context(), now)

	require.NoError(t, err)
	assert.Equal(t, []int64{fixtures.VandelayID}, summary.Frozen)
	assert.Contains(t, *env.DB.Workspace.GetX(t.Context(), fixtures.VandelayID).SuspensionReason, "news.vandelay.test")
}

func TestAFreezeIsAuditedAsTheSystemAndSendsNoWarning(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Now()
	lay(t, env, fixtures.VandelayID, "mail.vandelay.test", now, 2000, 0, 10)

	_, err := detector(t, env).Run(t.Context(), now)
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
	now := time.Now()
	lay(t, env, fixtures.VandelayID, "mail.vandelay.test", now, 2000, 0, 10)
	env.SystemMail.SetErr(assert.AnError)

	summary, err := detector(t, env).Run(t.Context(), now)

	require.NoError(t, err, "logged, not retried: a suspended Workspace is skipped from now on")
	assert.Equal(t, []int64{fixtures.VandelayID}, summary.Frozen)
	assert.True(t, suspended(t, env, fixtures.VandelayID))
}

func TestASuspendedWorkspaceIsSkippedAndKeepsItsFirstActorAndReason(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Now()
	_, err := suspension.SuspendWorkspace(t.Context(), env.Bus, fixtures.VandelayID, suspension.StaffMember("op-7"), "chargeback")
	require.NoError(t, err)
	lay(t, env, fixtures.VandelayID, "mail.vandelay.test", now, 2000, 0, 10)

	summary, err := detector(t, env).Run(t.Context(), now)

	require.NoError(t, err)
	assert.Equal(t, abuse.Summary{}, summary)
	ws := env.DB.Workspace.GetX(t.Context(), fixtures.VandelayID)
	assert.Equal(t, workspace.SuspendedByKindStaff, *ws.SuspendedByKind)
	assert.Equal(t, "chargeback", *ws.SuspensionReason)
	assert.Empty(t, env.SystemMail.Messages())
}

func TestTheDetectorNeverLiftsASuspension(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Now()
	lay(t, env, fixtures.VandelayID, "mail.vandelay.test", now, 2000, 0, 10)
	d := detector(t, env)
	_, err := d.Run(t.Context(), now)
	require.NoError(t, err)

	_, err = d.Run(t.Context(), now.Add(72*time.Hour)) // clean by then

	require.NoError(t, err)
	assert.True(t, suspended(t, env, fixtures.VandelayID))
	assert.True(t, suspended(t, env, fixtures.HooliID), "a suspension by anyone else stands too")
}

func TestAnExemptWorkspaceIsNeitherWarnedNorFrozenButCanBeSuspendedByHand(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Now()
	lay(t, env, fixtures.KramericaID, "mail.kramerica.test", now, 2000, 0, 100)

	summary, err := detector(t, env).Run(t.Context(), now)

	require.NoError(t, err)
	assert.Equal(t, abuse.Summary{}, summary)
	assert.False(t, suspended(t, env, fixtures.KramericaID))
	assert.Empty(t, env.SystemMail.Messages())
	changed, err := suspension.SuspendWorkspace(t.Context(), env.Bus, fixtures.KramericaID, suspension.CLI, "verified abuse")
	require.NoError(t, err)
	assert.True(t, changed, "an exemption is not a loophole for real abuse")
}

func TestAfterAnUnsuspendOnlyNewerEventsCountUntilTheFloorIsMetAgain(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Now()
	lay(t, env, fixtures.VandelayID, "mail.vandelay.test", now, 2000, 0, 10)
	d := detector(t, env)
	_, err := d.Run(t.Context(), now)
	require.NoError(t, err)
	_, err = suspension.UnsuspendWorkspace(t.Context(), env.Bus, fixtures.VandelayID, suspension.StaffMember("1"))
	require.NoError(t, err)

	immediately, err := d.Run(t.Context(), now.Add(time.Minute))
	require.NoError(t, err)
	assert.Empty(t, immediately.Frozen, "the same old complaints do not freeze a cleared sender again")

	later := now.Add(2 * time.Hour)
	lay(t, env, fixtures.VandelayID, "mail.vandelay.test", later, 999, 0, 99) // under the floor since the clearance
	underFloor, err := d.Run(t.Context(), later)
	require.NoError(t, err)
	assert.Empty(t, underFloor.Frozen)

	lay(t, env, fixtures.VandelayID, "mail.vandelay.test", later, 1, 0, 0) // the 1000th send
	refrozen, err := d.Run(t.Context(), later)
	require.NoError(t, err)
	assert.Equal(t, []int64{fixtures.VandelayID}, refrozen.Frozen)
}

func TestUnsuspendingRecordsTheClearance(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := suspension.UnsuspendWorkspace(t.Context(), env.Bus, fixtures.HooliID, suspension.StaffMember("1"))
	require.NoError(t, err)

	assert.NotNil(t, env.DB.Workspace.GetX(t.Context(), fixtures.HooliID).ClearedAt)
}

func TestTheDashboardWarningIsTheDetectorsOwnComputation(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Now()
	lay(t, env, fixtures.VandelayID, "mail.vandelay.test", now, 2000, 0, 3)
	d := detector(t, env)
	ws := env.DB.Workspace.GetX(t.Context(), fixtures.VandelayID)

	breach, err := d.Warning(t.Context(), env.DB.Scoped(fixtures.VandelayID), ws, now)

	require.NoError(t, err)
	assert.Equal(t, abuse.TierWarn, breach.Tier)
	assert.Equal(t, abuse.Complaint, breach.Metric)
	assert.Equal(t, "mail.vandelay.test", breach.Domain)
}

func TestAWarnedWorkspaceThatIsThenSuspendedByHandEndsItsEpisode(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Now()
	lay(t, env, fixtures.VandelayID, "mail.vandelay.test", now, 2000, 0, 3)
	d := detector(t, env)
	_, err := d.Run(t.Context(), now)
	require.NoError(t, err)
	require.NotNil(t, env.DB.Workspace.GetX(t.Context(), fixtures.VandelayID).AbuseWarningNotifiedAt)

	_, err = suspension.SuspendWorkspace(t.Context(), env.Bus, fixtures.VandelayID, suspension.CLI, "manual")
	require.NoError(t, err)
	_, err = d.Run(t.Context(), now.Add(time.Minute))

	require.NoError(t, err)
	assert.Nil(t, env.DB.Workspace.GetX(t.Context(), fixtures.VandelayID).AbuseWarningNotifiedAt)
}

func TestAWorkspaceInItsDeletionWindowIsNotJudged(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Now()
	lay(t, env, fixtures.VandelayID, "mail.vandelay.test", now, 2000, 0, 10)
	env.DB.Workspace.UpdateOneID(fixtures.VandelayID).SetDiscardedAt(now).ExecX(t.Context())

	summary, err := detector(t, env).Run(t.Context(), now)

	require.NoError(t, err)
	assert.Equal(t, abuse.Summary{}, summary)
	assert.False(t, suspended(t, env, fixtures.VandelayID))
	assert.Empty(t, env.SystemMail.Messages())
}
