package jobs_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestTelegramObservationIdentityIsIndependentForEachBotAndWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	botA, botB, otherWorkspace := env.AcmeTelegram(t), env.AcmeSecondTelegram(t), env.LacunaTelegram(t)
	for _, update := range []string{said(1, optOutUser, "/stop"), said(2, optOutUser, "/start")} {
		reply := env.PostTelegramUpdate(t, botA, update)
		require.Equal(t, http.StatusOK, reply.Code)
	}
	for _, bot := range []testhelper.TelegramBot{botB, otherWorkspace} {
		reply := env.PostTelegramUpdate(t, bot, said(1, optOutUser, "/stop"))
		require.Equal(t, http.StatusOK, reply.Code)
	}
	for _, workspaceID := range []int64{fixtures.AcmeID, fixtures.LacunaID} {
		stop, err := unsubscribes.Get(t.Context(), env.DB.Scoped(workspaceID), channels.Telegram, "4101", eligibility.SourceEverything)
		require.NoError(t, err)
		assert.NotNil(t, stop)
	}
}

func TestTelegramResumePreservesASourceScopedOptOut(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.LacunaTelegram(t)
	// Sid's fixture opt-out covers Broadcasts, independently of the Channel-wide stop.
	for _, update := range []string{said(1, 77407, "/stop"), said(2, 77407, "/start"), said(1, 77407, "/stop")} {
		reply := env.PostTelegramUpdate(t, bot, update)
		require.Equal(t, http.StatusOK, reply.Code)
	}
	scope := env.DB.Scoped(fixtures.LacunaID)
	source, err := unsubscribes.Get(t.Context(), scope, channels.Telegram, "77407", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.NotNil(t, source)
	everything, err := unsubscribes.Get(t.Context(), scope, channels.Telegram, "77407", eligibility.SourceEverything)
	notFound := ent.IsNotFound(err)
	require.True(t, notFound)
	assert.Nil(t, everything)
}

func TestTelegramStopFansOutToTheMessageAndVisitorOwnersOnlyOnTransitions(t *testing.T) {
	env := testhelper.Setup(t)
	botA := env.AcmeTelegram(t)
	// The received message names Liam; the chat now reaches Emma through her fixture Visitor.
	env.DB.OutboundMessage.UpdateOneID(fixtures.OutboundMessageParkedClaimID).
		SetStatus(outboundmessage.StatusSent).SetChannel(channels.Telegram).
		SetContactID(fixtures.ContactLiamID).SetDestination(fixtures.BindingEmmaPhoneDestination).
		SetIntegrationID(int64(botA.Integration.ID)).SetProviderMessageID("77").ExecX(t.Context())
	user := mustInt(t, fixtures.BindingEmmaPhoneDestination)
	for _, update := range []string{pressed(1, user, 77, "cb-1", "unsubscribe"), pressed(2, user, 77, "cb-2", "unsubscribe"), said(3, user, "/start"), pressed(2, user, 77, "cb-2", "unsubscribe")} {
		reply := env.PostTelegramUpdate(t, botA, update)
		require.Equal(t, http.StatusOK, reply.Code)
	}
	stopped := optedOut(t, env, user)
	assert.False(t, stopped)
	for _, contactID := range []int64{fixtures.ContactLiamID, fixtures.ContactEmmaID} {
		transitions := env.QueuedCount(t, systemevent.TelegramUnsubscribed, map[string]any{"contactId": contactID})
		assert.Equal(t, 1, transitions)
	}
	fresh := env.PostTelegramUpdate(t, botA, pressed(4, user, 77, "cb-4", "unsubscribe"))
	require.Equal(t, http.StatusOK, fresh.Code)
	stopped = optedOut(t, env, user)
	assert.True(t, stopped)
	for _, contactID := range []int64{fixtures.ContactLiamID, fixtures.ContactEmmaID} {
		transitions := env.QueuedCount(t, systemevent.TelegramUnsubscribed, map[string]any{"contactId": contactID})
		assert.Equal(t, 2, transitions)
	}
}
