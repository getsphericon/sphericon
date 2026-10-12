package external_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/jobs"
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
	assert.Equal(t, broadcastrecipient.StatusPending, r.Status)
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

	// recipients_total follows the recipient rows: one pending recipient removed from
	// each Broadcast, so reports' rates do not divide by recipients that are gone.
	assert.Equal(t, 0, solo.RecipientsTotal)
	assert.Equal(t, 1, still.RecipientsTotal)
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
	env.EnqueueJob(t, jobs.RunStepArgs{RunID: fixtures.AutomationRunErasableID})
	env.EnqueueJob(t, jobs.SendRecipientArgs{RecipientID: fixtures.BroadcastRecipientErasablePendingID, BroadcastID: fixtures.BroadcastInitechSendingID})
	env.EnqueueJob(t, jobs.SendRecipientArgs{RecipientID: fixtures.BroadcastRecipientBystanderPendingID, BroadcastID: fixtures.BroadcastInitechSendingID})

	eraseErasable(t, env)

	left := env.Outbox(t, events.NameEmailSent, events.NameContactCreated)
	require.Len(t, left, 1, "only the bystander's envelope is left")
	assert.Equal(t, fmt.Sprint(bystanderID), fmt.Sprint(left[0].Data["contactId"]))

	evaluate := env.JobsOf(t, jobs.EvaluateTriggerArgs{}.Kind())
	require.Len(t, evaluate, 1)
	assert.EqualValues(t, bystanderID, evaluate[0]["contact_id"])
	assert.Empty(t, env.JobsOf(t, jobs.RunStepArgs{}.Kind()))
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

	var left []string
	for _, args := range env.JobsOf(t, jobkind.DeliverWebhook) {
		left = append(left, fmt.Sprint(args["delivery_id"]))
	}
	assert.ElementsMatch(t, []string{"bystander", "nobody", "other-tenant"}, left)
}
