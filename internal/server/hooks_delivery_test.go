package server_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Delivery notices end to end for an SES Integration (ADR 0053): the notices in
// testdata/sns name the provider message id of fixtures OutboundMessageSesAlice, and the hook
// links each Event to that message and stamps its delivery result. The Preset's own `tests`
// hold the mapping of each kind.

// deliveryNames are every action a Delivery notice becomes.
var deliveryNames = []string{
	events.NameEmailDelivered, events.NameEmailDelayed, events.NameEmailDropped,
	events.NameEmailBounced, events.NameEmailComplained,
}

// sesAliceProviderMessageID is the provider message id of OutboundMessageSesAlice, the one the
// notices in testdata/sns name.
const sesAliceProviderMessageID = "ses-msg-alice-1"

func sesAliceMessage(t *testing.T, env *testhelper.TestEnv) *ent.OutboundMessage {
	t.Helper()
	m, err := env.DB.OutboundMessage.Get(t.Context(), fixtures.OutboundMessageSesAliceID)
	require.NoError(t, err)
	return m
}

func postNotice(t *testing.T, env *testhelper.TestEnv, name string) {
	t.Helper()
	rec := postSNS(t, env, fixtures.IntegrationAcmeSesIngestKey, snsNotification(t, name))
	require.Equal(t, http.StatusOK, rec.Code)
}

// A delivery notice for a sent message creates email.delivered linked to that message, and
// stamps it with the time the hook received it.
func TestSESDeliveryNoticeLinksAndStampsTheMessage(t *testing.T) {
	env := setupHooks(t)

	postNotice(t, env, "delivery")

	got := outboxEvents(t, env, fixtures.ContactAliceEmail)
	require.Len(t, got, 1)
	assert.Equal(t, events.NameEmailDelivered, got[0].Action)
	assert.EqualValues(t, fixtures.OutboundMessageSesAliceID, got[0].OutboundMessageID)
	assert.Equal(t, sesAliceProviderMessageID, got[0].ProviderMessageID)
	assert.Equal(t, "transactional", got[0].Surface)
	assert.EqualValues(t, fixtures.ContactAliceID, got[0].ContactID)
	assert.Equal(t, "sns-msg-20/"+fixtures.ContactAliceEmail, got[0].DedupID)

	m := sesAliceMessage(t, env)
	require.NotNil(t, m.DeliveredAt)
	assert.True(t, hookClock.Equal(*m.DeliveredAt))
	assert.Nil(t, m.BouncedAt)
	assert.Nil(t, m.ComplainedAt)
}

// A redelivered notice persists one Event and changes nothing on the message; so does a
// later notice of the same kind: the first stamp stays.
func TestSESDeliveryStampIsSetOnce(t *testing.T) {
	env := setupHooks(t)
	first := hookClock.Add(-time.Hour)
	require.NoError(t, env.DB.OutboundMessage.UpdateOneID(fixtures.OutboundMessageSesAliceID).SetDeliveredAt(first).Exec(t.Context()))

	postNotice(t, env, "delivery")
	postNotice(t, env, "delivery")
	persistOutbox(t, env)

	m := sesAliceMessage(t, env)
	require.NotNil(t, m.DeliveredAt)
	assert.True(t, first.Equal(*m.DeliveredAt), "the first stamp is never replaced")
	n, err := env.DB.Event.Query().Where(event.SourceID("sns-msg-20/" + fixtures.ContactAliceEmail)).Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

// A delay and a drop are Events with a reason, linked to the message, and stamp nothing.
func TestSESDelayAndDropAreEventsWithAReasonAndStampNothing(t *testing.T) {
	env := setupHooks(t)

	postNotice(t, env, "delay")
	postNotice(t, env, "drop")

	byAction := map[string]events.EmailDelivery{}
	for _, ev := range outboxEvents(t, env, fixtures.ContactAliceEmail) {
		byAction[ev.Action] = ev
	}
	require.Len(t, byAction, 2)
	assert.Equal(t, "smtp; 451 4.7.1 try again later", byAction[events.NameEmailDelayed].Reason)
	assert.Equal(t, "Bad content", byAction[events.NameEmailDropped].Reason)
	for _, ev := range byAction {
		assert.EqualValues(t, fixtures.OutboundMessageSesAliceID, ev.OutboundMessageID)
	}
	m := sesAliceMessage(t, env)
	assert.Nil(t, m.DeliveredAt)
	assert.Nil(t, m.BouncedAt)
	assert.Nil(t, m.ComplainedAt)
}

// A notice that names no message of ours keeps working at the level of the address.
func TestSESNoticeWithoutAMatchingMessageStaysAtTheAddress(t *testing.T) {
	env := setupHooks(t)

	postNotice(t, env, "delivery-unmatched")
	rec := postSNS(t, env, fixtures.IntegrationAcmeSesIngestKey, snsNotification(t, "permanent-bounce"))
	require.Equal(t, http.StatusOK, rec.Code)

	got := outboxEvents(t, env, fixtures.ContactAliceEmail)
	require.Len(t, got, 2)
	for _, ev := range got {
		assert.Zero(t, ev.OutboundMessageID)
	}
	assert.Equal(t, "ses-msg-unknown", got[0].ProviderMessageID)
	assert.Nil(t, sesAliceMessage(t, env).DeliveredAt)
	applySuppression(t, env)
	assert.True(t, suppressed(t, env, fixtures.ContactAliceEmail), "the permanent bounce still suppresses")
}

// A permanent bounce stamps the message as bounced and a complaint stamps it as
// complained, both still suppress.
func TestSESBounceAndComplaintStampTheirKindAndSuppress(t *testing.T) {
	env := setupHooks(t)

	postNotice(t, env, "bounce-for-message")
	m := sesAliceMessage(t, env)
	require.NotNil(t, m.BouncedAt)
	assert.Nil(t, m.ComplainedAt)
	assert.Nil(t, m.DeliveredAt)
	bounces := outboxEvents(t, env, fixtures.ContactAliceEmail)
	require.Len(t, bounces, 1)
	assert.Equal(t, "smtp; 550 5.1.1 user unknown", bounces[0].Reason)
	assert.Equal(t, events.BounceKindPermanent, bounces[0].BounceKind)

	postNotice(t, env, "complaint-for-message")
	assert.NotNil(t, sesAliceMessage(t, env).ComplainedAt)

	applySuppression(t, env)
	assert.True(t, suppressed(t, env, fixtures.ContactAliceEmail))
}

// Delivery, delay, drop and a transient bounce never suppress.
func TestSESOnlyPermanentBouncesAndComplaintsSuppress(t *testing.T) {
	env := setupHooks(t)

	for _, name := range []string{"delivery", "delay", "drop", "transient-bounce"} {
		postNotice(t, env, name)
	}
	applySuppression(t, env)

	assert.False(t, suppressed(t, env, fixtures.ContactAliceEmail))
	assert.False(t, suppressed(t, env, "soft@example.com"))
}
