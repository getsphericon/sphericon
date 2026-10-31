package external_test

import (
	"context"
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
	"github.com/getsphericon/sphericon/ent/outboxmessage"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestExternalContactsEraseRemovesUnsentBroadcastRecipients(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	eraseErasable(t, env)

	for _, id := range []int64{fixtures.BroadcastRecipientErasablePendingID, fixtures.BroadcastRecipientErasableSoloID} {
		ok, err := env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.ID(id)).Exist(ctx)
		require.NoError(t, err)
		assert.Falsef(t, ok, "unsent recipient %d is gone", id)
	}
	// The bystander's pending recipient is untouched, and so is the sent row (it
	// stays as an anonymous delivery record, see the erasure matrix test).
	r, err := env.DB.BroadcastRecipient.Get(ctx, fixtures.BroadcastRecipientBystanderPendingID)
	require.NoError(t, err)
	stillPending, err := env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.ID(r.ID), broadcasts.Pending()).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, stillPending)
	ok, err := env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.ID(fixtures.BroadcastRecipientErasableID)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, ok)

	// A Broadcast left with nothing pending settles; one with a pending recipient keeps sending.
	solo, err := env.DB.Broadcast.Get(ctx, fixtures.BroadcastInitechSendingSoloID)
	require.NoError(t, err)
	assert.Equal(t, broadcast.StatusSent, solo.Status)
	still, err := env.DB.Broadcast.Get(ctx, fixtures.BroadcastInitechSendingID)
	require.NoError(t, err)
	assert.Equal(t, broadcast.StatusSending, still.Status)

	// The figures follow the recipient rows: one pending recipient removed from each
	// Broadcast, so reports' rates do not divide by recipients that are gone.
	figures, err := broadcasts.StatsOf(ctx, env.DB.Scoped(fixtures.InitechID), solo.ID, still.ID)
	require.NoError(t, err)
	assert.Zero(t, figures[solo.ID].Recipients)
	assert.Equal(t, 1, figures[still.ID].Recipients)
}

func TestExternalContactsEraseClearsOutboxAndJobArguments(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	var erasedID, bystanderID int64 = fixtures.ContactErasableID, fixtures.ContactBystanderID
	require.NoError(t, env.Bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		for _, ev := range []events.DomainEvent{
			&events.EmailEngagement{Action: events.NameEmailSent, WorkspaceID: fixtures.InitechID, ContactID: erasedID, Email: "erin@initech.test"},
			&events.ContactCreated{WorkspaceID: fixtures.InitechID, Email: "Erin@Initech.test"}, // by address only
			&events.EmailEngagement{Action: events.NameEmailSent, WorkspaceID: fixtures.InitechID, ContactID: bystanderID, Email: "ben@initech.test"},
		} {
			if err := pub.Publish(ctx, ev); err != nil {
				return err
			}
		}
		return nil
	}))
	require.Len(t, env.Outbox(t, events.NameEmailSent, events.NameContactCreated), 3)

	env.EnqueueJob(t, jobs.EvaluateTriggerArgs{WorkspaceID: fixtures.InitechID, ContactID: erasedID, Action: "x"})
	env.EnqueueJob(t, jobs.EvaluateTriggerArgs{WorkspaceID: fixtures.InitechID, ContactID: bystanderID, Action: "x"})
	env.EnqueueJob(t, stepping.RunStepArgs{RunID: fixtures.AutomationRunErasableID})
	env.EnqueueJob(t, jobs.SendRecipientArgs{RecipientID: fixtures.BroadcastRecipientErasablePendingID, BroadcastID: fixtures.BroadcastInitechSendingID})
	env.EnqueueJob(t, jobs.SendRecipientArgs{RecipientID: fixtures.BroadcastRecipientBystanderPendingID, BroadcastID: fixtures.BroadcastInitechSendingID})

	eraseErasable(t, env)

	left := env.Outbox(t, events.NameEmailSent, events.NameContactCreated)
	require.Len(t, left, 1, "only the bystander's envelope is left")
	assert.Equal(t, strconv.FormatInt(bystanderID, 10), fmt.Sprint(left[0].Data["contactId"]))

	evaluate := env.JobsOf(t, jobs.EvaluateTriggerArgs{}.Kind())
	require.Len(t, evaluate, 1)
	assert.EqualValues(t, bystanderID, evaluate[0]["contact_id"])
	assert.Empty(t, env.JobsOf(t, stepping.RunStepArgs{}.Kind()))
	recipients := env.JobsOf(t, jobs.SendRecipientArgs{}.Kind())
	require.Len(t, recipients, 1)
	assert.EqualValues(t, fixtures.BroadcastRecipientBystanderPendingID, recipients[0]["recipient_id"])
}

func TestExternalContactsEraseClearsQueuedWebhookDeliveries(t *testing.T) {
	env := testhelper.Setup(t)

	delivery := func(endpoint int64, id, body string) {
		env.EnqueueJob(t, jobs.DeliverWebhookArgs{
			EndpointID: endpoint, EventName: "email.sent", DeliveryID: id, Body: []byte(body),
		})
	}
	// Queued deliveries of the erased Contact's events, by contact id and by address
	// (in either case), next to the bystander's, a payload naming nobody, and a Globex
	// delivery for the same address, which is another tenant's and must survive.
	delivery(fixtures.WebhookInitechID, "by-top-level-id", fmt.Sprintf(`{"contactId":%d,"data":{}}`, fixtures.ContactErasableID))
	delivery(fixtures.WebhookInitechID, "by-data-id", fmt.Sprintf(`{"data":{"contactId":%d}}`, fixtures.ContactErasableID))
	delivery(fixtures.WebhookInitechID, "by-address", `{"data":{"email":"Erin@Initech.test"}}`)
	delivery(fixtures.WebhookInitechID, "bystander", fmt.Sprintf(`{"contactId":%d,"data":{"email":"ben@initech.test"}}`, fixtures.ContactBystanderID))
	delivery(fixtures.WebhookInitechID, "nobody", `{"data":{}}`)
	delivery(fixtures.WebhookGlobexID, "other-tenant", `{"data":{"email":"erin@initech.test"}}`)

	eraseErasable(t, env)

	left := lo.Map(env.JobsOf(t, jobkind.DeliverWebhook.Kind), func(args map[string]any, _ int) string {
		return fmt.Sprint(args["delivery_id"])
	})
	assert.ElementsMatch(t, []string{"bystander", "nobody", "other-tenant"}, left)
}

func TestExternalContactsEraseClearsQueuedHTTPStepCallsAndFailureExcerpts(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// A queued call of the erased Contact's runs (one mid-sequence, one failed), next to the
	// bystander's and another tenant's.
	for _, id := range []int64{
		fixtures.AutomationRunErasableID, fixtures.AutomationRunErasableHTTPFailedID,
		fixtures.AutomationRunBystanderHTTPFailedID, fixtures.AutomationRunHTTPFailedID,
	} {
		env.EnqueueJob(t, stepping.RunHTTPStepArgs{RunID: id})
	}

	eraseErasable(t, env)

	left := lo.Map(env.JobsOf(t, jobkind.HTTPStep.Kind), func(args map[string]any, _ int) int64 {
		return int64(args["run_id"].(float64))
	})
	assert.ElementsMatch(t, []int64{fixtures.AutomationRunBystanderHTTPFailedID, fixtures.AutomationRunHTTPFailedID}, left)

	// The failure excerpt lives on the run: it goes with the Enrollment, others keep theirs.
	gone, err := env.DB.AutomationRun.Query().Where(automationrun.ID(fixtures.AutomationRunErasableHTTPFailedID)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, gone)
	kept, err := env.DB.AutomationRun.Get(ctx, fixtures.AutomationRunBystanderHTTPFailedID)
	require.NoError(t, err)
	require.NotNil(t, kept.FailureExcerpt)
	assert.Equal(t, "ben@initech.test rejected", *kept.FailureExcerpt)
}

// A dead letter naming the erased Contact, by id or by address, goes with its outbox envelopes.
func TestExternalContactsEraseClearsDeadLetters(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	require.NoError(t, env.Bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		for _, ev := range []events.DomainEvent{
			&events.EmailEngagement{Action: events.NameEmailSent, WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactErasableID, Email: "erin@initech.test"},
			&events.ContactCreated{WorkspaceID: fixtures.InitechID, Email: "Erin@Initech.test"},
			&events.EmailEngagement{Action: events.NameEmailSent, WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactBystanderID, Email: "ben@initech.test"},
		} {
			if err := pub.Publish(ctx, ev); err != nil {
				return err
			}
		}
		return nil
	}))
	for _, row := range env.DB.OutboxMessage.Query().AllX(ctx) {
		// test writes: a dead letter is written by the router's poison queue, which no test of
		// Erasure runs; this one copies the envelopes as the router would.
		env.DB.OutboxDeadLetter.Create().SetUUID(row.UUID).SetCreatedAt(row.CreatedAt).SetTransactionID(row.TransactionID).
			SetPayload(row.Payload).SetMetadata(row.Metadata).ExecX(ctx)
	}

	eraseErasable(t, env)

	left := env.DB.OutboxDeadLetter.Query().AllX(ctx)
	require.Len(t, left, 1, "only the bystander's dead letter is left")
	assert.Contains(t, string(left[0].Payload), "ben@initech.test")
}

// An outbox row published before the envelope moved to the metadata (the whole envelope as its
// payload) that names the erased Contact goes too.
func TestExternalContactsEraseClearsOutboxRowsOfThePreviousShape(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	for i, data := range []string{
		fmt.Sprintf(`{"contactId":%d,"email":"erin@initech.test"}`, fixtures.ContactErasableID),
		`{"email":"Erin@Initech.test"}`,
		fmt.Sprintf(`{"contactId":%d,"email":"ben@initech.test"}`, fixtures.ContactBystanderID),
	} {
		// test writes: a row of the previous shape is only ever written by the previous version.
		env.DB.OutboxMessage.Create().SetUUID(fmt.Sprintf("old-%d", i)).SetCreatedAt(time.Now()).SetTransactionID("1").
			SetPayload([]byte(fmt.Sprintf(`{"id":"old-%d","name":"email.sent","version":1,"workspaceId":%d,"data":%s}`, i, fixtures.InitechID, data))).
			ExecX(ctx)
	}

	eraseErasable(t, env)

	left := env.DB.OutboxMessage.Query().Where(outboxmessage.UUIDHasPrefix("old-")).AllX(ctx)
	require.Len(t, left, 1, "only the bystander's row is left")
	assert.Contains(t, string(left[0].Payload), "ben@initech.test")
}
