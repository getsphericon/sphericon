package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/stepexecution"
)

// holdClock is the clock the Staff hold read uses, moved by the test.
type holdClock struct{ t time.Time }

func (c *holdClock) now() time.Time { return c.t }

// heldMod is an Outbound module whose send gate reads Staff holds on the test clock, as the
// running application's does.
func heldMod(env *testhelper.TestEnv, clock *holdClock, resolver outboundmessages.Senders) *outboundmessages.Module {
	return outboundmessages.New(env.Bus, holds.New(func() bool { return true }, clock.now), resolver, messagetoken.New("test-secret", "http://local"))
}

func setHold(t *testing.T, env *testhelper.TestEnv, expires time.Time) {
	t.Helper()
	env.ExtendHold(t.Context(), fixtures.AcmeID, fixtures.HoldAcmeLapsedID, expires)
}

// A hold mid-Broadcast pauses it with its remaining recipients still pending, nobody is
// failed or consumed, and it resumes by itself once the hold lapses.
func TestAnStaffHoldMidBroadcastPausesRecipientsAndResumesOnItsOwn(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	clock := &holdClock{t: time.Now()}
	fs := &fakeSender{}
	mod := heldMod(env, clock, resolvingTo(fs))

	ids := []int64{
		env.PlaceRecipient(ctx, fixtures.BroadcastSendingID, fixtures.ContactAliceID),
		env.PlaceRecipient(ctx, fixtures.BroadcastSendingID, fixtures.ContactCarolID),
	}
	require.NoError(t, deliverRecipient(ctx, env.DB, mod, ids[0]))

	setHold(t, env, clock.t.Add(time.Hour))
	err := deliverRecipient(ctx, env.DB, mod, ids[1])
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

	require.NoError(t, stepexecution.OnEvent(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), fixtures.ContactHoldDemoID, automations.Event{Action: "hold_demo", OccurredAt: time.Now()}, 0))
	runIDs := enrollmentsOf(t, env, fixtures.AutomationHoldDemoID, fixtures.ContactHoldDemoID)
	require.Len(t, runIDs, 1)
	setHold(t, env, clock.t.Add(time.Hour))

	held := stepperWith(env, heldMod(env, clock, resolvingTo(fs)), stepexecution.WithClock(clock.now))
	require.True(t, env.RunNextStep(t, held))
	run := env.DB.AutomationRun.GetX(ctx, runIDs[0])
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Zero(t, run.CurrentStep)
	assert.Equal(t, []map[string]any{stepJob(runIDs[0], 0)}, queuedSteps(t, env), "the same step is queued again")
	assert.Empty(t, fs.sent)

	clock.t = clock.t.Add(2 * time.Hour)
	env.RunQueuedSteps(t, held)
	assert.Len(t, fs.sent, 1, "the step sends once the hold has lapsed")
}
