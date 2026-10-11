package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mokevnin/sphericon/ent/automation"
	"github.com/mokevnin/sphericon/ent/automationrun"
	"github.com/mokevnin/sphericon/ent/broadcast"
	"github.com/mokevnin/sphericon/ent/broadcastrecipient"
	"github.com/mokevnin/sphericon/internal/dnstest"
	"github.com/mokevnin/sphericon/internal/fixtures"
	"github.com/mokevnin/sphericon/internal/jobs"
	"github.com/mokevnin/sphericon/internal/messaging/registry"
	"github.com/mokevnin/sphericon/internal/suspension"
	"github.com/mokevnin/sphericon/internal/testhelper"
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
	a, err := env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).
		SetName("Branches").SetTriggerEvent("branch.test").SetStatus(automation.StatusActive).
		SetDefinition(definition).Save(ctx)
	require.NoError(t, err)
	run, err := env.DB.AutomationRun.Create().SetAutomationID(a.ID).SetContactID(c.ID).SetWorkspaceID(fixtures.AcmeID).Save(ctx)
	require.NoError(t, err)
	return run.ID
}

func runStatus(ctx context.Context, t *testing.T, env *testhelper.TestEnv, id int64) automationrun.Status {
	t.Helper()
	return env.DB.AutomationRun.GetX(ctx, id).Status
}

func TestRunStepOutcomes(t *testing.T) {
	ctx := context.Background()

	t.Run("a finished run is done without work", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := enroll(ctx, t, env, "["+emailStep+"]", true)
		env.DB.AutomationRun.UpdateOneID(id).SetStatus(automationrun.StatusCompleted).ExecX(ctx)
		res, err := jobs.RunStep(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), id)
		require.NoError(t, err)
		assert.True(t, res.Done)
	})

	t.Run("a run past its last step completes", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := enroll(ctx, t, env, "[]", true)
		res, err := jobs.RunStep(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), id)
		require.NoError(t, err)
		assert.True(t, res.Done)
		assert.Equal(t, automationrun.StatusCompleted, runStatus(ctx, t, env, id))
	})

	t.Run("an undecodable definition fails the run", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := enroll(ctx, t, env, "not json", true)
		_, err := jobs.RunStep(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), id)
		require.Error(t, err)
		assert.Equal(t, automationrun.StatusFailed, runStatus(ctx, t, env, id))
	})

	t.Run("an unknown step type fails the run", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := enroll(ctx, t, env, `[{"type":"teleport"}]`, true)
		_, err := jobs.RunStep(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), id)
		require.ErrorContains(t, err, "unknown step type")
		assert.Equal(t, automationrun.StatusFailed, runStatus(ctx, t, env, id))
	})

	t.Run("a contact without an email completes the run", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := enroll(ctx, t, env, "["+emailStep+"]", false)
		res, err := jobs.RunStep(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), id)
		require.NoError(t, err)
		assert.True(t, res.Done)
		assert.Equal(t, automationrun.StatusCompleted, runStatus(ctx, t, env, id))
	})

	t.Run("a template that cannot render fails the run", func(t *testing.T) {
		env := testhelper.Setup(t)
		broken := `{"type":"email","subject":"{% if %}broken","body":"<mjml><mj-body></mj-body></mjml>"}`
		id := enroll(ctx, t, env, "["+broken+"]", true)
		_, err := jobs.RunStep(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), id)
		require.Error(t, err)
		assert.Equal(t, automationrun.StatusFailed, runStatus(ctx, t, env, id))
	})

	t.Run("a held workspace waits and asks again later", func(t *testing.T) {
		env := testhelper.Setup(t)
		id := enroll(ctx, t, env, "["+emailStep+"]", true)
		_, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.System, "complaints")
		require.NoError(t, err)
		fs := &fakeSender{}
		res, err := jobs.RunStep(ctx, env.DB, newMod(env, resolvingTo(fs)), id)
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
		res, err := jobs.RunStep(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), 424242)
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
		_, cerr := env.DB.Automation.Create().SetWorkspaceID(fixtures.AcmeID).SetName(name).
			SetTriggerEvent(trigger).SetStatus(status).SetDefinition("[" + emailStep + "]").Save(ctx)
		require.NoError(t, cerr)
	}
	mk("active match", "trigger.match", automation.StatusActive)
	mk("draft match", "trigger.match", automation.StatusDraft)
	mk("other trigger", "trigger.other", automation.StatusActive)

	ids, err := jobs.EvaluateTrigger(ctx, env.DB.Scoped(fixtures.AcmeID), c.ID, "trigger.match")
	require.NoError(t, err)
	assert.Len(t, ids, 1)
}

func TestPlanBroadcastFailsOnABrokenSegment(t *testing.T) {
	ctx := context.Background()

	t.Run("segment of another workspace", func(t *testing.T) {
		env := testhelper.Setup(t)
		env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetSegmentID(fixtures.SegmentGlobexID).ExecX(ctx)
		_, err := jobs.PlanBroadcast(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), fixtures.BroadcastDraftID)
		require.Error(t, err)
		assert.Equal(t, broadcast.StatusFailed, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Status)
	})

	t.Run("segment with an undecodable definition", func(t *testing.T) {
		env := testhelper.Setup(t)
		env.DB.Segment.UpdateOneID(fixtures.SegmentProPlanID).SetDefinition("{not json").ExecX(ctx)
		env.DB.Broadcast.UpdateOneID(fixtures.BroadcastDraftID).SetSegmentID(fixtures.SegmentProPlanID).ExecX(ctx)
		_, err := jobs.PlanBroadcast(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), fixtures.BroadcastDraftID)
		require.Error(t, err)
		assert.Equal(t, broadcast.StatusFailed, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Status)
	})
}

func TestSendToRecipientOutcomes(t *testing.T) {
	ctx := context.Background()
	plan := func(t *testing.T, env *testhelper.TestEnv) (ids []int64) {
		t.Helper()
		ids, err := jobs.PlanBroadcast(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), fixtures.BroadcastDraftID)
		require.NoError(t, err)
		require.NotEmpty(t, ids)
		return ids
	}
	status := func(env *testhelper.TestEnv, id int64) broadcastrecipient.Status {
		return env.DB.BroadcastRecipient.GetX(ctx, id).Status
	}

	t.Run("a contact that lost its email fails terminally", func(t *testing.T) {
		env := testhelper.Setup(t)
		ids := plan(t, env)
		rec := env.DB.BroadcastRecipient.GetX(ctx, ids[0])
		env.DB.Contact.UpdateOneID(rec.ContactID).ClearEmail().ExecX(ctx)
		require.NoError(t, jobs.SendToRecipient(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), ids[0]))
		assert.Equal(t, broadcastrecipient.StatusFailed, status(env, ids[0]))
	})

	t.Run("a suppression after planning skips the recipient", func(t *testing.T) {
		env := testhelper.Setup(t)
		ids := plan(t, env)
		rec := env.DB.BroadcastRecipient.GetX(ctx, ids[0])
		c := env.DB.Contact.GetX(ctx, rec.ContactID)
		env.DB.Suppression.Create().SetWorkspaceID(fixtures.AcmeID).SetChannel("email").
			SetDestination(*c.Email).SetReason("bounce").ExecX(ctx)
		require.NoError(t, jobs.SendToRecipient(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), ids[0]))
		assert.Equal(t, broadcastrecipient.StatusSkipped, status(env, ids[0]))
	})

	t.Run("a held workspace leaves the recipient pending", func(t *testing.T) {
		env := testhelper.Setup(t)
		ids := plan(t, env)
		_, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.System, "complaints")
		require.NoError(t, err)
		err = jobs.SendToRecipient(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), ids[0])
		require.Error(t, err)
		var held *jobs.HeldError
		assert.ErrorAs(t, err, &held)
		assert.Equal(t, broadcastrecipient.StatusPending, status(env, ids[0]))
	})

	// A recipient that is gone was erased with its Contact (ADR 0021) after its job
	// was queued: nothing to send, nothing to retry.
	t.Run("unknown recipient", func(t *testing.T) {
		env := testhelper.Setup(t)
		require.NoError(t, jobs.SendToRecipient(ctx, env.DB, newMod(env, resolvingTo(&fakeSender{})), 424242))
	})
}

// A failing platform sender surfaces from the owner notifications (the caller logs
// it), while a healthy one reaches the owner.
func TestOwnerNotificationsSurfaceSenderErrors(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	_, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.System, "complaints")
	require.NoError(t, err)
	env.SystemMail.SetErr(errors.New("smtp down"))

	require.Error(t, jobs.NotifySendingDomainUnverified(ctx, env.DB, env.SystemMail, fixtures.SendingDomainVerifiedID))
	require.Error(t, jobs.NotifyWorkspaceSuspended(ctx, env.DB, env.SystemMail, fixtures.AcmeID))
	require.Error(t, jobs.NotifySendingDomainUnverified(ctx, env.DB, env.SystemMail, 424242), "unknown domain")
}

// The inline adapter notifies the owner when a re-check flips a domain to
// unverified, and a failed notification never fails the check.
func TestInlineSendingDomainVerifyNotifiesOnFlip(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	gone := dnstest.Resolver(t, nil).LookupTXT
	inline := jobs.NewInline(env.DB, newMod(env, resolvingTo(&fakeSender{})), env.SystemMail, gone, envCipher(t), registry.Default(), "http://local")

	require.NoError(t, inline.EnqueueSendingDomainVerify(ctx, fixtures.SendingDomainVerifiedID))
	require.Len(t, env.SystemMail.Messages(), 1)

	// Already unverified now: no second alert.
	require.NoError(t, inline.EnqueueSendingDomainVerify(ctx, fixtures.SendingDomainVerifiedID))
	assert.Len(t, env.SystemMail.Messages(), 1)

	// A failing alert on a fresh flip is only logged.
	env.DB.SendingDomain.UpdateOneID(fixtures.SendingDomainVerifiedID).SetVerified(true).ExecX(ctx)
	env.SystemMail.SetErr(errors.New("smtp down"))
	require.NoError(t, inline.EnqueueSendingDomainVerify(ctx, fixtures.SendingDomainVerifiedID))

	require.Error(t, inline.EnqueueSendingDomainVerify(ctx, 424242))
}
