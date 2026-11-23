package jobs_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/order"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The credit of an Order names the Outbound message whose click earned it (ADR 0064, ADR 0083),
// beside the Broadcast or Automation: a person follows a tracked link of a message, the shop
// reports an Order of theirs, and the orders job credits it. No surface shows the message yet, so
// the Order is read where the credit is kept.

// orderAfterClick is an Order Completed of the Contact in the Workspace, worked by the orders job
// as one the shop sent, and the Order it made.
func orderAfterClick(t *testing.T, env *testhelper.TestEnv, workspaceID, contactID int64) *ent.Order {
	t.Helper()
	publishEvent(t, env, &events.CollectedEvent{
		WorkspaceID: workspaceID, ContactID: contactID, SubjectID: "buyer", Action: orders.ActionCompleted,
		Properties: map[string]any{"order_id": "CLICKED-1", "total": 20.0, "currency": "USD"},
	})
	env.PerformEnqueuedJobs(t, jobs.OrderEventArgs{})
	o, err := env.DB.Scoped(workspaceID).Order().Query().Where(order.OrderID("CLICKED-1")).Only(t.Context())
	require.NoError(t, err)
	return o
}

// Alice clicks a link of the Broadcast email she was sent.
func TestAnOrderAfterAClickOnABroadcastEmailNamesTheClickedEmail(t *testing.T) {
	env := testhelper.Setup(t)
	link, err := env.Tracker.ClickURL(env.Tracked(t, fixtures.OutboundMessageTrackingSampleID), "https://dest.test/x")
	require.NoError(t, err)
	_, token, _ := strings.Cut(link, "/e/c/")
	resp := follow(t, env, "/e/c/"+token, browserUA)
	require.Equal(t, http.StatusFound, resp.Code)

	o := orderAfterClick(t, env, fixtures.AcmeID, fixtures.ContactAliceID)

	require.NotNil(t, o.BroadcastID)
	assert.EqualValues(t, fixtures.BroadcastSentUntrackedID, *o.BroadcastID)
	assert.Nil(t, o.AutomationID)
	require.NotNil(t, o.CreditedMessageID)
	assert.EqualValues(t, fixtures.OutboundMessageTrackingSampleID, *o.CreditedMessageID)
}

// Bea follows a short link of the SMS Broadcast she was sent.
func TestAnOrderAfterAClickOnAnSMSShortLinkNamesTheClickedText(t *testing.T) {
	env := testhelper.Setup(t)
	env.LacunaTwilio(t)
	id := int64(fixtures.BroadcastLacunaSmsLinksID)
	require.NoError(t, env.SendBroadcastThrough(t, env.OutboundModule(), id))
	paths := smsShortPaths(t, env)
	require.NotEmpty(t, paths)
	resp := follow(t, env, paths[0], browserUA)
	require.Equal(t, http.StatusFound, resp.Code)
	text, err := env.DB.OutboundMessage.Query().Where(outboundmessage.BroadcastID(id)).Only(t.Context())
	require.NoError(t, err)

	o := orderAfterClick(t, env, fixtures.LacunaID, fixtures.ContactLacunaBeaID)

	require.NotNil(t, o.BroadcastID)
	assert.Equal(t, id, *o.BroadcastID)
	require.NotNil(t, o.CreditedMessageID)
	assert.Equal(t, text.ID, *o.CreditedMessageID)
}

// Bob follows the short link of the text of the Telegram message an Automation sent him.
func TestAnOrderAfterAClickOnATelegramShortLinkNamesTheClickedMessage(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	automationID := int64(fixtures.AutomationJobsTelegramLinkAndButtonID)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 701001, 701001)
	queueStep(t, env, fixtures.AutomationRunJobsTelegramLinkAndButtonBobID)
	runSteps(t, env)
	text, _ := shortPaths(t, env, "701001")
	require.NotEmpty(t, text)
	resp := follow(t, env, text[0], browserUA)
	require.Equal(t, http.StatusFound, resp.Code)
	sent, err := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationID(automationID), outboundmessage.ContactID(fixtures.ContactBobID)).Only(t.Context())
	require.NoError(t, err)

	o := orderAfterClick(t, env, fixtures.AcmeID, fixtures.ContactBobID)

	assert.Nil(t, o.BroadcastID)
	require.NotNil(t, o.AutomationID)
	assert.Equal(t, automationID, *o.AutomationID)
	require.NotNil(t, o.CreditedMessageID)
	assert.Equal(t, sent.ID, *o.CreditedMessageID)
}
