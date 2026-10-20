package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// holdClock is the clock the Staff hold read uses, moved by the test.
type holdClock struct{ t time.Time }

func (c *holdClock) now() time.Time { return c.t }

// heldMod is an Outbound module whose send gate reads Staff holds on the test clock, as the
// running application's does.
func heldMod(env *testhelper.TestEnv, clock *holdClock, resolver outbound.Senders) *outbound.Module {
	return outbound.New(env.Bus, holds.New(holds.Always, clock.now), resolver, tracking.New("test-secret", "http://local"))
}

func setHold(t *testing.T, env *testhelper.TestEnv, key string, expires time.Time) {
	t.Helper()
	_, err := env.DB.Scoped(fixtures.AcmeID).Hold().Create().
		SetKey(key).SetReasonCode("review").SetMessage("Sending is paused for now.").SetSetBy("service-a").SetExpiresAt(expires).
		Save(t.Context())
	require.NoError(t, err)
}

// A hold mid-Broadcast pauses it with its remaining recipients still pending, nobody is
// failed or consumed, and it resumes by itself once the hold lapses.
func TestAnStaffHoldMidBroadcastPausesRecipientsAndResumesOnItsOwn(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	clock := &holdClock{t: time.Now()}
	fs := &fakeSender{}
	mod := heldMod(env, clock, resolvingTo(fs))

	ids, err := planAudience(ctx, env.DB, mod, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(ids), 2)
	require.NoError(t, deliverRecipient(ctx, env.DB, mod, ids[0]))

	setHold(t, env, "k1", clock.t.Add(time.Hour))
	err = deliverRecipient(ctx, env.DB, mod, ids[1])
	var held *broadcasts.HeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, holds.ReasonStaff, held.Reason)
	assert.True(t, stillPending(ctx, env.DB, ids[1]), "the recipient is neither failed nor consumed")
	assert.Len(t, fs.sent, 1)

	clock.t = clock.t.Add(2 * time.Hour)
	require.NoError(t, deliverRecipient(ctx, env.DB, mod, ids[1]), "the retry goes through with no one clearing anything")
	assert.False(t, stillPending(ctx, env.DB, ids[1]))
	assert.Len(t, fs.sent, 2)
}

func TestAnStaffHoldMakesAnAutomationStepWaitAndContinue(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	clock := &holdClock{t: time.Now()}
	fs := &fakeSender{}

	runIDs, err := jobs.EvaluateTrigger(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.ContactHoldDemoID, "hold_demo")
	require.NoError(t, err)
	require.Len(t, runIDs, 1)
	setHold(t, env, "k1", clock.t.Add(time.Hour))

	res, err := jobs.RunStep(ctx, env.DB, noFence, heldMod(env, clock, resolvingTo(fs)), runIDs[0])
	require.NoError(t, err)
	assert.False(t, res.Done)
	require.NotNil(t, res.ResumeAt)
	run := env.DB.AutomationRun.GetX(ctx, runIDs[0])
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Zero(t, run.CurrentStep)
	assert.Empty(t, fs.sent)

	clock.t = clock.t.Add(2 * time.Hour)
	_, err = jobs.RunStep(ctx, env.DB, noFence, heldMod(env, clock, resolvingTo(fs)), runIDs[0])
	require.NoError(t, err)
	assert.Len(t, fs.sent, 1, "the step sends once the hold has lapsed")
}
