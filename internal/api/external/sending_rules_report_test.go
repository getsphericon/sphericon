package external_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// skippedBy is every Skipped reason of the contract at zero but the ones counted.
func skippedBy(counted map[externalapi.SkippedReason]int32) []externalapi.SkippedReasonCount {
	return lo.Map(externalapi.SkippedReason("").AllValues(), func(r externalapi.SkippedReason, _ int) externalapi.SkippedReasonCount {
		return externalapi.SkippedReasonCount{Reason: r, Count: counted[r]}
	})
}

// waitingFor is every Deferral cause of the contract at zero but the ones counted.
func waitingFor(counted map[externalapi.DeferralCause]int32) []externalapi.DeferralCauseCount {
	return lo.Map(externalapi.DeferralCause("").AllValues(), func(c externalapi.DeferralCause, _ int) externalapi.DeferralCauseCount {
		return externalapi.DeferralCauseCount{Cause: c, Count: counted[c]}
	})
}

// A sending Broadcast's report counts its Recipients Skipped by reason and its pending ones waiting
// now by cause (ADR 0178): Alice is suppressed, so Send-eligibility skips her; Carol finds the Send
// cap spent and waits for it. Every other reason and cause reads zero.
func TestExternalBroadcastReportCountsTheSkippedByReasonAndTheWaitingByCause(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	alice := env.PlaceRecipient(ctx, fixtures.BroadcastSendingID, fixtures.ContactAliceID)
	carol := env.PlaceRecipient(ctx, fixtures.BroadcastSendingID, fixtures.ContactCarolID)
	env.SuppressDestination(ctx, fixtures.ContactAliceEmail)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(1).ExecX(ctx)
	env.DB.SendCapLimiter.UpdateOneID(fixtures.SendCapLimiterAcmeID).SetDayTokens(0).SetRefilledAt(time.Now()).ExecX(ctx)
	del := testhelper.Module[*broadcasts.Delivery](env)
	require.NoError(t, del.SendRecipient(ctx, s, alice))
	var deferred *broadcasts.DeferredError
	require.ErrorAs(t, del.SendRecipient(ctx, s, carol), &deferred)

	report, err := env.ExternalScoped(t, apitokens.ScopeBroadcastsRead).BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: entityIDString(fixtures.BroadcastSendingID)})
	require.NoError(t, err)

	assert.Equal(t, skippedBy(map[externalapi.SkippedReason]int32{externalapi.SkippedReasonSuppressed: 1}), report.SkippedByReason)
	assert.Equal(t, waitingFor(map[externalapi.DeferralCause]int32{externalapi.DeferralCauseSendCap: 1}), report.WaitingByCause)
}

// An Automation's send Step counts its messages Skipped by reason and the active Enrollments whose
// send waits there now by cause (ADR 0178): the Hold demo's Contact waits on the Provider limit a
// receipt spent, Alice, suppressed, is Skipped by Send-eligibility.
func TestExternalAutomationReportCountsEachStepsSkippedByReasonAndItsWaitingByCause(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerDay(1).ExecX(ctx)
	receipt, err := env.OutboundModule().Send(ctx, acme, outboundmessages.Request{
		Ref: outboundmessages.TransactionalRef{}, Key: "tx:receipt", Destination: "a@example.com",
		Content: outboundmessages.EmailContent{Subject: "Receipt", Body: "<p>Hi</p>"},
	})
	require.NoError(t, err)
	require.Equal(t, outboundmessages.Sent, receipt.Outcome)
	env.SuppressDestination(ctx, fixtures.ContactAliceEmail)
	stepper := unfencedStepper(env)
	for _, contactID := range []int64{fixtures.ContactAliceID, fixtures.ContactHoldDemoID} {
		require.NoError(t, stepper.OnEvent(ctx, acme, automations.OnEventInput{ContactID: contactID, Event: automations.Event{Action: "hold_demo", OccurredAt: time.Now()}}))
	}
	require.True(t, env.RunNextStep(t, stepper))
	require.True(t, env.RunNextStep(t, stepper))
	waiting := env.DB.AutomationRun.Query().Where(automationrun.AutomationID(fixtures.AutomationHoldDemoID), automationrun.ContactID(fixtures.ContactHoldDemoID)).OnlyX(ctx)
	step := env.StepIDAt(t, waiting.ID, 0)

	report := automationReport(t, env.ExternalScoped(t, apitokens.ScopeAutomationsRead), externalapi.AutomationsReportParams{ID: entityIDString(fixtures.AutomationHoldDemoID)})

	assert.Equal(t, []externalapi.AutomationStepFigures{{
		StepId: externalapi.StepId(step), Skipped: 1,
		SkippedByReason: skippedBy(map[externalapi.SkippedReason]int32{externalapi.SkippedReasonSuppressed: 1}),
		WaitingByCause:  waitingFor(map[externalapi.DeferralCause]int32{externalapi.DeferralCauseProviderLimit: 1}),
	}}, report.Steps)
}

// unfencedStepper is the Step execution over the Instance's Outbound send, without the fence a send
// takes on a pool connection of its own: the test's database is one transaction on one connection.
func unfencedStepper(env *testhelper.TestEnv) *automations.StepExecution {
	unfenced := func(ctx context.Context, _ *ent.Scoped, _ int64, effect func(context.Context) error) error {
		return effect(ctx)
	}
	return automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), unfenced, env.Cipher, nil, time.Now)
}

// An SMS Broadcast's report counts the sends a Sending window judged by a single zone (ADR 0134):
// the Spring sale's one text went to a Los Angeles number, in one zone. An email Broadcast has no
// window and no such count.
func TestExternalBroadcastReportCountsTheSendsJudgedByASingleZone(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, apitokens.ScopeBroadcastsRead)

	sms, err := c.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: entityIDString(fixtures.BroadcastSmsSentID)})
	require.NoError(t, err)
	email, err := c.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: entityIDString(fixtures.BroadcastSendingID)})
	require.NoError(t, err)

	assert.Equal(t, externalapi.NewNilInt32(1), sms.SingleZoneCount)
	assert.True(t, email.SingleZoneCount.Null)
}
