package contacterasure_test

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The work still queued for the subject is cancelled by the Erasure, in its transaction.

func TestAnErasureCancelsTheContactsUnsentBroadcastRecipients(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	eraseErasable(t, env)

	for _, id := range []int64{fixtures.BroadcastRecipientErasablePendingID, fixtures.BroadcastRecipientErasableSoloID} {
		ok := env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.ID(id)).ExistX(ctx)
		assert.Falsef(t, ok, "unsent recipient %d is gone", id)
	}
	// The bystander's pending recipient is untouched, and so is the sent row (it stays as an
	// anonymous delivery record).
	stillPending := env.DB.BroadcastRecipient.Query().
		Where(broadcastrecipient.ID(fixtures.BroadcastRecipientBystanderPendingID), broadcasts.Pending()).ExistX(ctx)
	assert.True(t, stillPending)
	assert.True(t, env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.ID(fixtures.BroadcastRecipientErasableID)).ExistX(ctx))
	// A Broadcast left with nothing pending settles; one with a pending recipient keeps sending.
	solo := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastInitechSendingSoloID)
	assert.Equal(t, broadcast.StatusSent, solo.Status)
	still := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastInitechSendingID)
	assert.Equal(t, broadcast.StatusSending, still.Status)
	// The figures follow the recipient rows: one pending recipient removed from each Broadcast, so
	// reports' rates do not divide by recipients that are gone.
	figures, err := testhelper.Module[*broadcasts.Module](env).StatsOf(ctx, env.DB.Scoped(fixtures.InitechID), solo.ID, still.ID)
	require.NoError(t, err)
	assert.Zero(t, figures[solo.ID].Recipients)
	assert.Equal(t, 1, figures[still.ID].Recipients)
}

func TestAnErasureCancelsTheQueuedJobsThatNameTheContact(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	var erasedID, bystanderID int64 = fixtures.ContactErasableID, fixtures.ContactBystanderID
	require.NoError(t, env.Bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		for _, ev := range []events.DomainEvent{
			&events.EmailEngagement{Action: systemevent.EmailSent, WorkspaceID: fixtures.InitechID, ContactID: erasedID, Email: fixtures.ContactErasableEmail},
			&events.ContactCreated{WorkspaceID: fixtures.InitechID, Email: "Erin@Initech.test"}, // by address only
			&events.EmailEngagement{Action: systemevent.EmailSent, WorkspaceID: fixtures.InitechID, ContactID: bystanderID, Email: fixtures.ContactBystanderEmail},
		} {
			if err := pub.Publish(ctx, ev); err != nil {
				return err
			}
		}
		return nil
	}))
	require.Len(t, env.QueuedMessages(t, systemevent.EmailSent, systemevent.ContactCreated), 3)
	env.EnqueueJob(t, jobs.EvaluateTriggerArgs{WorkspaceID: fixtures.InitechID, ContactID: erasedID, Action: "x", OccurredAt: time.Now(), SourceID: "evt-1"})
	env.EnqueueJob(t, jobs.EvaluateTriggerArgs{WorkspaceID: fixtures.InitechID, ContactID: bystanderID, Action: "x", OccurredAt: time.Now(), SourceID: "evt-2"})
	env.EnqueueJob(t, automations.RunStepArgs{WorkspaceID: fixtures.InitechID, RunID: fixtures.AutomationRunErasableID})
	env.EnqueueJob(t, jobs.SendRecipientArgs{WorkspaceID: fixtures.InitechID, RecipientID: fixtures.BroadcastRecipientErasablePendingID, BroadcastID: fixtures.BroadcastInitechSendingID})
	env.EnqueueJob(t, jobs.SendRecipientArgs{WorkspaceID: fixtures.InitechID, RecipientID: fixtures.BroadcastRecipientBystanderPendingID, BroadcastID: fixtures.BroadcastInitechSendingID})

	eraseErasable(t, env)

	left := env.QueuedMessages(t, systemevent.EmailSent, systemevent.ContactCreated)
	require.Len(t, left, 1, "only the bystander's envelope is left")
	assert.Equal(t, strconv.FormatInt(bystanderID, 10), fmt.Sprint(left[0].Data["contactId"]))
	evaluate := env.JobsOf(t, jobs.EvaluateTriggerArgs{}.Kind())
	require.Len(t, evaluate, 2, "the bystander's: the one his email.sent queued and the one put here")
	for _, args := range evaluate {
		assert.EqualValues(t, bystanderID, args["contact_id"])
	}
	assert.Empty(t, env.JobsOf(t, automations.RunStepArgs{}.Kind()))
	recipients := env.JobsOf(t, jobs.SendRecipientArgs{}.Kind())
	require.Len(t, recipients, 1)
	assert.EqualValues(t, fixtures.BroadcastRecipientBystanderPendingID, recipients[0]["recipient_id"])
}

func TestAnErasureCancelsTheQueuedWebhookDeliveriesThatNameTheContact(t *testing.T) {
	env := testhelper.Setup(t)
	delivery := func(workspace, endpoint int64, id, body string) {
		env.EnqueueJob(t, jobs.DeliverWebhookArgs{
			WorkspaceID: workspace, EndpointID: endpoint, EventName: systemevent.EmailSent, DeliveryID: id, Body: []byte(body),
		})
	}
	// Queued deliveries of the erased Contact's events, by contact id and by address (in either
	// case), next to the bystander's, a payload naming nobody, and a Globex delivery for the same
	// address, which is another tenant's and must survive.
	delivery(fixtures.InitechID, fixtures.WebhookInitechID, "by-top-level-id", fmt.Sprintf(`{"contactId":%q,"data":{}}`, testhelper.ID(fixtures.ContactErasableID)))
	delivery(fixtures.InitechID, fixtures.WebhookInitechID, "by-data-id", fmt.Sprintf(`{"data":{"contactId":%q}}`, testhelper.ID(fixtures.ContactErasableID)))
	delivery(fixtures.InitechID, fixtures.WebhookInitechID, "by-address", `{"data":{"email":"Erin@Initech.test"}}`)
	delivery(fixtures.InitechID, fixtures.WebhookInitechID, "bystander", fmt.Sprintf(`{"contactId":%q,"data":{"email":%q}}`, testhelper.ID(fixtures.ContactBystanderID), fixtures.ContactBystanderEmail))
	delivery(fixtures.InitechID, fixtures.WebhookInitechID, "nobody", `{"data":{}}`)
	delivery(fixtures.GlobexID, fixtures.WebhookGlobexID, "other-tenant", fmt.Sprintf(`{"data":{"email":%q}}`, fixtures.ContactErasableEmail))

	eraseErasable(t, env)

	left := lo.Map(env.JobsOf(t, jobkind.DeliverWebhook.Kind), func(args map[string]any, _ int) string {
		return fmt.Sprint(args["delivery_id"])
	})
	assert.ElementsMatch(t, []string{"bystander", "nobody", "other-tenant"}, left)
}

func TestAnErasureCancelsTheQueuedHTTPStepCallsOfTheContactsRuns(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	// A queued call of the erased Contact's runs (one mid-sequence, one failed), next to the
	// bystander's and another tenant's.
	for _, id := range []int64{
		fixtures.AutomationRunErasableID, fixtures.AutomationRunErasableHTTPFailedID, fixtures.AutomationRunBystanderHTTPFailedID,
	} {
		env.EnqueueJob(t, automations.RunHTTPStepArgs{WorkspaceID: fixtures.InitechID, RunID: id})
	}
	env.EnqueueJob(t, automations.RunHTTPStepArgs{WorkspaceID: fixtures.AcmeID, RunID: fixtures.AutomationRunHTTPFailedID})

	eraseErasable(t, env)

	left := lo.Map(env.JobsOf(t, jobkind.HTTPStep.Kind), func(args map[string]any, _ int) int64 {
		return int64(args["run_id"].(float64))
	})
	assert.ElementsMatch(t, []int64{fixtures.AutomationRunBystanderHTTPFailedID, fixtures.AutomationRunHTTPFailedID}, left)
	// The failure excerpt lives on the run: it goes with the Enrollment, others keep theirs.
	assert.False(t, env.DB.AutomationRun.Query().Where(automationrun.ID(fixtures.AutomationRunErasableHTTPFailedID)).ExistX(ctx))
	kept := env.DB.AutomationRun.GetX(ctx, fixtures.AutomationRunBystanderHTTPFailedID)
	require.NotNil(t, kept.FailureExcerpt)
	assert.Equal(t, fixtures.ContactBystanderEmail+" rejected", *kept.FailureExcerpt)
}

// An erased Contact leaves no Firing and no queued evaluation of an Event of theirs; the
// bystander's stay.
func TestAnErasureCancelsTheContactsRuleFiringsAndQueuedEvaluations(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.EnqueueJob(t, jobs.EvaluateRulesArgs{WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactErasableID, Action: "rule_initech", SourceID: "evt-1"})
	env.EnqueueJob(t, jobs.EvaluateRulesArgs{WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactBystanderID, Action: "rule_initech", SourceID: "evt-2"})

	eraseErasable(t, env)

	assert.False(t, env.DB.RuleFiring.Query().Where(rulefiring.ContactID(fixtures.ContactErasableID)).ExistX(ctx))
	assert.True(t, env.DB.RuleFiring.Query().Where(rulefiring.ID(fixtures.RuleFiringBystanderID)).ExistX(ctx))
	queued := env.JobsOf(t, jobkind.RuleEvaluate.Kind)
	require.Len(t, queued, 1)
	assert.EqualValues(t, fixtures.ContactBystanderID, queued[0]["contact_id"])
}
