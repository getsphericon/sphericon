package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/domainverify"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/messaging/registry"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// enroll creates an active single-definition automation and an enrollment of a
// fresh email contact (or an email-less one) in it, returning the run id.
func enroll(ctx context.Context, t *testing.T, env *testhelper.TestEnv, definition string, withEmail bool) int64 {
	t.Helper()
	cc := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetFirstName("Br")
	if withEmail {
		cc.SetEmail("branch@test.dev")
	} else {
		cc.SetSubjectID("branch-anon")
	}
	c, err := cc.Save(ctx)
	require.NoError(t, err)
	a, err := testhelper.AutomationWithSteps(ctx, env.DB, env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Branches").SetTriggerEvent("branch.test").SetStatus(automation.StatusActive), definition)
	require.NoError(t, err)
	versionID, err := testhelper.LatestVersionID(ctx, env.DB, a.ID)
	require.NoError(t, err)
	run, err := env.DB.AutomationRun.Create().SetAutomationID(a.ID).SetVersionID(versionID).SetContactID(c.ID).SetWorkspaceID(fixtures.AcmeID).Save(ctx)
	require.NoError(t, err)
	return run.ID
}

func runStatus(ctx context.Context, t *testing.T, env *testhelper.TestEnv, id int64) automationrun.Status {
	t.Helper()
	return env.DB.AutomationRun.GetX(ctx, id).Status
}

func TestRunStepOutcomes(t *testing.T) {
	ctx := context.Background()

	t.Run("a finished run stays finished and the step ends quietly", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := enroll(ctx, t, env, "[]", true)
		env.DB.AutomationRun.UpdateOneID(id).SetStatus(automationrun.StatusFailed).ExecX(ctx)
		res, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), id)
		require.NoError(t, err)
		assert.True(t, res.Done)
		assert.Equal(t, automationrun.StatusFailed, runStatus(ctx, t, env, id))
	})

	t.Run("a run past its last step completes", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := enroll(ctx, t, env, "[]", true)
		res, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), id)
		require.NoError(t, err)
		assert.True(t, res.Done)
		assert.Equal(t, automationrun.StatusCompleted, runStatus(ctx, t, env, id))
	})

	t.Run("an undecodable definition fails the run", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := enroll(ctx, t, env, "not json", true)
		_, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), id)
		require.Error(t, err)
		assert.Equal(t, automationrun.StatusFailed, runStatus(ctx, t, env, id))
	})

	t.Run("an unknown step type fails the run", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := enroll(ctx, t, env, `[{"type":"teleport"}]`, true)
		_, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), id)
		require.ErrorContains(t, err, "unknown step type")
		assert.Equal(t, automationrun.StatusFailed, runStatus(ctx, t, env, id))
	})

	t.Run("a contact without an email completes the run", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := enroll(ctx, t, env, "["+emailStep+"]", false)
		res, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), id)
		require.NoError(t, err)
		assert.True(t, res.Done)
		assert.Equal(t, automationrun.StatusCompleted, runStatus(ctx, t, env, id))
	})

	t.Run("a template that cannot render fails the run", func(t *testing.T) {
		env := testhelper.Setup(t)
		broken := `{"type":"email","subject":"{% if %}broken","body":"<p></p>"}`
		id := enroll(ctx, t, env, "["+broken+"]", true)
		_, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), id)
		require.Error(t, err)
		assert.Equal(t, automationrun.StatusFailed, runStatus(ctx, t, env, id))
	})

	t.Run("a held workspace waits and asks again later", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := enroll(ctx, t, env, "["+emailStep+"]", true)
		_, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.System, "complaints")
		require.NoError(t, err)
		fs := &fakeSender{}
		res, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(fs)), id)
		require.NoError(t, err)
		assert.False(t, res.Done)
		require.NotNil(t, res.ResumeAt)
		assert.True(t, res.ResumeAt.After(time.Now().Add(10*time.Minute)))
		assert.Empty(t, fs.sent)
		assert.Equal(t, automationrun.StatusActive, runStatus(ctx, t, env, id), "the enrollment is unchanged")
	})

	// A run that is gone was erased with its Contact (ADR 0021) after its job was
	// queued: the job is finished, not retried.
	t.Run("an unknown run is finished", func(t *testing.T) {
		env := testhelper.Setup(t)
		res, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), 424242)
		require.NoError(t, err)
		assert.True(t, res.Done)
	})
}

func TestEvaluateTriggerEnrollsOnlyMatchingActiveAutomations(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail("trigger@test.dev").Save(ctx)
	require.NoError(t, err)
	mk := func(name, trigger string, status automation.Status) {
		_, cerr := testhelper.AutomationWithSteps(ctx, env.DB, env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).SetName(name).
			SetTriggerEvent(trigger).SetStatus(status), "["+emailStep+"]")
		require.NoError(t, cerr)
	}
	mk("active match", "trigger.match", automation.StatusActive)
	mk("draft match", "trigger.match", automation.StatusDraft)
	mk("other trigger", "trigger.other", automation.StatusActive)

	ids, err := jobs.EvaluateTrigger(ctx, env.DB.Scoped(fixtures.AcmeID), c.ID, "trigger.match")
	require.NoError(t, err)
	assert.Len(t, ids, 1)
}

// A failing platform sender surfaces from the owner notifications (the caller logs
// it), while a healthy one reaches the owner.
func TestOwnerNotificationsSurfaceSenderErrors(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	_, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.System, "complaints")
	require.NoError(t, err)
	env.SystemMail.SetErr(errors.New("smtp down"))

	require.Error(t, suspension.NotifyOwners(ctx, env.DB, accounts.OwnerNotifier(env.DB, env.SystemMail), "", fixtures.AcmeID))
}

// The inline adapter notifies the owner when a re-check flips a domain to
// unverified, and a failed notification never fails the check.
func TestInlineSendingDomainVerifyNotifiesOnFlip(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	gone := dnstest.Resolver(t, nil).LookupTXT
	inline := jobs.NewInline(env.DB, newMod(env, resolvingTo(&fakeSender{})), env.SystemMail, domainverify.New(env.DB, gone, nil, accounts.OwnerNotifier(env.DB, env.SystemMail)), envCipher(t), registry.Default(), "http://local")

	require.NoError(t, inline.EnqueueSendingDomainVerify(ctx, fixtures.SendingDomainVerifiedID))
	require.Len(t, env.SystemMail.Messages(), 1)

	// Already unverified now: no second alert.
	require.NoError(t, inline.EnqueueSendingDomainVerify(ctx, fixtures.SendingDomainVerifiedID))
	assert.Len(t, env.SystemMail.Messages(), 1)

	// A failing alert on a fresh flip is only logged.
	env.DB.SendingDomain.UpdateOneID(fixtures.SendingDomainVerifiedID).SetVerified(true).SetVerifiedAt(time.Now()).ExecX(ctx)
	env.SystemMail.SetErr(errors.New("smtp down"))
	require.NoError(t, inline.EnqueueSendingDomainVerify(ctx, fixtures.SendingDomainVerifiedID))

	require.Error(t, inline.EnqueueSendingDomainVerify(ctx, 424242))
}
