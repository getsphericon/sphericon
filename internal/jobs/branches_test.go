package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func runStatus(ctx context.Context, t *testing.T, env *testhelper.TestEnv, id int64) automationrun.Status {
	t.Helper()
	return env.DB.AutomationRun.GetX(ctx, id).Status
}

func TestRunStepOutcomes(t *testing.T) {
	ctx := context.Background()

	t.Run("a finished run stays finished and the step ends quietly", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := int64(fixtures.AutomationRunJobsEmptyStepsID)
		env.DB.AutomationRun.UpdateOneID(id).SetStatus(automationrun.StatusFailed).ExecX(ctx)
		res, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), id)
		require.NoError(t, err)
		assert.True(t, res.Done)
		assert.Equal(t, automationrun.StatusFailed, runStatus(ctx, t, env, id))
	})

	t.Run("a run past its last step completes", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := int64(fixtures.AutomationRunJobsEmptyStepsID)
		res, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), id)
		require.NoError(t, err)
		assert.True(t, res.Done)
		assert.Equal(t, automationrun.StatusCompleted, runStatus(ctx, t, env, id))
	})

	t.Run("an undecodable definition fails the run", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := int64(fixtures.AutomationRunJobsUndecodableID)
		// The stored steps are corrupted here: a fixture must stay decodable (key rotation reads it).
		env.DB.AutomationVersion.UpdateOneID(fixtures.AutomationJobsUndecodableVersionID).SetSteps("not json").ExecX(ctx)
		_, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), id)
		require.Error(t, err)
		assert.Equal(t, automationrun.StatusFailed, runStatus(ctx, t, env, id))
	})

	t.Run("an unknown step type fails the run", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := int64(fixtures.AutomationRunJobsUnknownStepID)
		_, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), id)
		require.ErrorContains(t, err, "unknown step type")
		assert.Equal(t, automationrun.StatusFailed, runStatus(ctx, t, env, id))
	})

	t.Run("a contact without an email completes the run", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := int64(fixtures.AutomationRunJobsNoEmailID)
		res, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), id)
		require.NoError(t, err)
		assert.True(t, res.Done)
		assert.Equal(t, automationrun.StatusCompleted, runStatus(ctx, t, env, id))
	})

	t.Run("a template that cannot render fails the run", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := int64(fixtures.AutomationRunJobsBrokenTemplateID)
		_, err := jobs.RunStep(ctx, env.DB, noFence, newMod(env, resolvingTo(&fakeSender{})), id)
		require.Error(t, err)
		assert.Equal(t, automationrun.StatusFailed, runStatus(ctx, t, env, id))
	})

	t.Run("a held workspace waits and asks again later", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := int64(fixtures.AutomationRunJobsPendingID)
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

	// The greeting's trigger is shared by an active and a draft automation; Bob is in neither.
	ids, err := jobs.EvaluateTrigger(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), fixtures.ContactBobID, triggerOf(t, env, fixtures.AutomationJobsGreetingID), 0)

	require.NoError(t, err)
	require.Len(t, ids, 1)
	assert.Equal(t, int64(fixtures.AutomationJobsGreetingID), env.DB.AutomationRun.GetX(ctx, ids[0]).AutomationID)
	assert.False(t, env.DB.AutomationRun.Query().Where(automationrun.AutomationID(fixtures.AutomationJobsGreetingDraftID)).ExistX(ctx), "the draft enrolled nobody")
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
	env.DB.SendingDomain.UpdateOneID(fixtures.SendingDomainVerifiedID).SetVerifiedAt(time.Now()).ClearVerificationLostAt().ExecX(ctx)
	env.SystemMail.SetErr(errors.New("smtp down"))
	require.NoError(t, inline.EnqueueSendingDomainVerify(ctx, fixtures.SendingDomainVerifiedID))

	require.Error(t, inline.EnqueueSendingDomainVerify(ctx, 424242))
}
