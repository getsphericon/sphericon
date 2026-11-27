package needsattentionreport_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A wait for the Sending window or the Frequency cap, and a send either Skipped, are a normal
// effect of the sending rules, not trouble (ADR 0178): Acme's report lists what it listed before
// a Recipient of its sending Broadcast waited for its window, an Enrollment waited at its email
// Step for the cap, and a message was Skipped for the cap.
func TestTheSendingRulesWaitsAndSkipsNeedNoAttention(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	before := itemsOf(t, env, fixtures.AcmeID)

	carol := env.PlaceRecipient(ctx, fixtures.BroadcastSendingID, fixtures.ContactCarolID)
	require.NoError(t, acme.BroadcastRecipient().UpdateOneID(carol).
		SetDeferredUntil(time.Now().Add(8*time.Hour)).SetDeferralCause(deferralcause.SendingWindow).Exec(ctx))
	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(ctx, acme, automations.OnEventInput{
		ContactID: fixtures.ContactHoldDemoID, Event: automations.Event{Action: "hold_demo", OccurredAt: time.Now()},
	}))
	require.NoError(t, acme.AutomationRun().Update().
		Where(automationrun.AutomationID(fixtures.AutomationHoldDemoID), automationrun.StatusEQ(automationrun.StatusActive)).
		SetDeferredSince(time.Now()).SetDeferralCause(deferralcause.FrequencyCap).Exec(ctx))
	require.NoError(t, acme.OutboundMessage().UpdateOneID(fixtures.OutboundMessageSkippedID).
		SetSkippedReason(skipreason.FrequencyCap).Exec(ctx))

	after := itemsOf(t, env, fixtures.AcmeID)

	assert.Equal(t, before, after)
}
