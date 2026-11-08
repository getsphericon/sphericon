package jobs_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The broadcasts consumer counts an opt-out of any Channel on the Broadcast it names (ADR 0113): an
// email.unsubscribed, a telegram.unsubscribed or an sms.unsubscribed of Bob from a message of the
// archived Broadcast, whose parked recipient he is.

func TestTheBroadcastCountsAnOptOutOfEveryChannelFromItsEvent(t *testing.T) {
	optOuts := map[string]events.DomainEvent{
		events.NameEmailUnsubscribed: &events.EmailEngagement{
			Action: events.NameEmailUnsubscribed, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactBobID,
			Email: fixtures.ContactBobEmail, BroadcastID: fixtures.BroadcastArchivedID,
		},
		events.NameTelegramUnsubscribed: &events.TelegramUnsubscribed{
			WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactBobID, Email: fixtures.ContactBobEmail,
			Via: events.UnsubscribedByButton, BroadcastID: fixtures.BroadcastArchivedID, DedupID: "test:telegram",
		},
		events.NameSmsUnsubscribed: &events.SmsUnsubscribed{
			WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactBobID, Email: fixtures.ContactBobEmail,
			Phone: "+14155550122", Via: events.UnsubscribedByLink, BroadcastID: fixtures.BroadcastArchivedID, DedupID: "test:sms",
		},
	}
	for name, ev := range optOuts {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			publishEvent(t, env, ev)
			queued := env.EventJobs(t, jobs.BroadcastUnsubscribeEventArgs{}.Kind())

			env.PerformEnqueuedJobs(t, jobs.BroadcastUnsubscribeEventArgs{})

			assert.Len(t, queued, 1, "the opt-out is handed to the broadcasts consumer")
			counted := env.DB.BroadcastRecipient.GetX(t.Context(), fixtures.BroadcastRecipientParkedBobID).UnsubscribedAt
			assert.NotNil(t, counted)
		})
	}
}

// An open and a click share the email engagement's type with an opt-out and name the Broadcast
// too; they are no opt-out, and the consumer never sees them.
func TestAnOpenOrAClickOfABroadcastsEmailIsNoUnsubscribe(t *testing.T) {
	env := testhelper.Setup(t)
	publishEvent(t, env, engagementOfBob(events.NameEmailOpened))
	publishEvent(t, env, engagementOfBob(events.NameEmailClicked))

	queued := env.EventJobs(t, jobs.BroadcastUnsubscribeEventArgs{}.Kind())
	assert.Empty(t, queued)
}

// A Webhook endpoint that selects sms.unsubscribed receives it, with its public body.
func TestAnEndpointThatSelectsSmsUnsubscribedReceivesIt(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.WebhookEndpoint.UpdateOneID(fixtures.WebhookCodebasicsID).SetEventTypes([]string{events.NameSmsUnsubscribed}).ExecX(t.Context())
	publishEvent(t, env, &events.SmsUnsubscribed{
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactTexterID, Email: fixtures.ContactTexterEmail,
		Phone: fixtures.ContactTexterPhone, Via: events.UnsubscribedByKeyword, DedupID: "test:sms",
	})

	env.PerformEnqueuedJobs(t, jobs.WebhookEventArgs{})

	d := deliveryTo(t, env, fixtures.WebhookCodebasicsID)
	assert.Equal(t, events.NameSmsUnsubscribed, d.EventName)
	body := bodyOf(t, d)
	assert.Equal(t, events.NameSmsUnsubscribed, body["type"])
	assert.EqualValues(t, fixtures.ContactTexterID, body["contactId"])
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, events.UnsubscribedByKeyword, data["via"])
}

// engagementOfBob is Bob's email engagement of the archived Broadcast's email.
func engagementOfBob(action string) *events.EmailEngagement {
	return &events.EmailEngagement{
		Action: action, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactBobID,
		Email: fixtures.ContactBobEmail, BroadcastID: fixtures.BroadcastArchivedID, DedupID: "test:" + action,
	}
}
