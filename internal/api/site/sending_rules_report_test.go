package site_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// skippedBy is every Skipped reason of the contract at zero but the ones counted.
func skippedBy(counted map[siteapi.SkippedReason]int32) []siteapi.SkippedReasonCount {
	return lo.Map(siteapi.SkippedReason("").AllValues(), func(r siteapi.SkippedReason, _ int) siteapi.SkippedReasonCount {
		return siteapi.SkippedReasonCount{Reason: r, Count: counted[r]}
	})
}

// waitingFor is every Deferral cause of the contract at zero but the ones counted.
func waitingFor(counted map[siteapi.DeferralCause]int32) []siteapi.DeferralCauseCount {
	return lo.Map(siteapi.DeferralCause("").AllValues(), func(c siteapi.DeferralCause, _ int) siteapi.DeferralCauseCount {
		return siteapi.DeferralCauseCount{Cause: c, Count: counted[c]}
	})
}

// A sending Broadcast's figures count its Recipients Skipped by reason and its pending ones waiting
// now by cause (ADR 0178): Alice is suppressed, so Send-eligibility skips her; Carol finds the Send
// cap spent and waits for it. Every other reason and cause reads zero.
func TestTheBroadcastFiguresCountTheSkippedByReasonAndTheWaitingByCause(t *testing.T) {
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

	got, err := env.SiteActor(t, fixtures.MemberMaryEmail).SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{
		WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.BroadcastSendingID),
	})
	require.NoError(t, err)

	assert.Equal(t, skippedBy(map[siteapi.SkippedReason]int32{siteapi.SkippedReasonSuppressed: 1}), got.Stats.SkippedByReason)
	assert.Equal(t, waitingFor(map[siteapi.DeferralCause]int32{siteapi.DeferralCauseSendCap: 1}), got.Stats.WaitingByCause)
}

// A Broadcast no longer sending has no Recipient waiting: one deferred when it failed waits for
// nothing, and every cause reads zero.
func TestAFinishedBroadcastHasNoneWaiting(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	carol := env.PlaceRecipient(ctx, fixtures.BroadcastSendingID, fixtures.ContactCarolID)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(1).ExecX(ctx)
	env.DB.SendCapLimiter.UpdateOneID(fixtures.SendCapLimiterAcmeID).SetDayTokens(0).SetRefilledAt(time.Now()).ExecX(ctx)
	var deferred *broadcasts.DeferredError
	require.ErrorAs(t, testhelper.Module[*broadcasts.Delivery](env).SendRecipient(ctx, s, carol), &deferred)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	_, err := c.SiteBroadcastsCancel(ctx, siteapi.SiteBroadcastsCancelParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.BroadcastSendingID)})
	require.NoError(t, err)

	got, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{
		WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.BroadcastSendingID),
	})
	require.NoError(t, err)

	assert.Equal(t, waitingFor(nil), got.Stats.WaitingByCause)
}

// An Automation's send Step counts its messages Skipped by reason and the active Enrollments whose
// send waits there now by cause (ADR 0178). Acme's default Integration lets one email out a day and
// a receipt took it: the Hold demo's Contact waits at the email Step on the Provider limit, while
// Alice, suppressed, is Skipped there by Send-eligibility and moves on.
func TestTheAutomationReportCountsEachStepsSkippedByReasonAndItsWaitingByCause(t *testing.T) {
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

	report, err := env.SiteActor(t, fixtures.MemberMaryEmail).SiteAutomationReportsGet(ctx, siteapi.SiteAutomationReportsGetParams{
		WorkspaceId: fixtures.AcmePublicID, AutomationId: idStr(fixtures.AutomationHoldDemoID),
	})
	require.NoError(t, err)

	figures := siteapi.AutomationStepFigures{
		StepId: siteapi.StepId(step), Skipped: 1,
		SkippedByReason: skippedBy(map[siteapi.SkippedReason]int32{siteapi.SkippedReasonSuppressed: 1}),
		WaitingByCause:  waitingFor(map[siteapi.DeferralCause]int32{siteapi.DeferralCauseProviderLimit: 1}),
	}
	assert.Equal(t, []siteapi.AutomationStepFigures{figures}, report.Steps)
	assert.Equal(t, []siteapi.AutomationVersionStepFigures{{
		Version: 1, StepId: figures.StepId, Skipped: 1,
		SkippedByReason: figures.SkippedByReason, WaitingByCause: figures.WaitingByCause,
	}}, report.VersionSteps)
}

// unfencedStepper is the Step execution over the Instance's Outbound send, without the fence a send
// takes on a pool connection of its own: the test's database is one transaction on one connection.
func unfencedStepper(env *testhelper.TestEnv) *automations.StepExecution {
	unfenced := func(ctx context.Context, _ *ent.Scoped, _ int64, effect func(context.Context) error) error {
		return effect(ctx)
	}
	return automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), unfenced, env.Cipher, nil, time.Now)
}
