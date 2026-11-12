package jobs_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"

	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestMessageProjectionsKeepEarliestObservationsDespiteHandlingOrderAndWorkerClock(t *testing.T) {
	first := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	later := first.Add(time.Minute)
	now := later
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return now }))
	publish := func(observed time.Time) {
		t.Helper()
		for _, action := range []string{systemevent.EmailOpened, systemevent.EmailClicked} {
			require.NoError(t, env.Bus.Publish(t.Context(), &events.EmailEngagement{
				Action: action, WorkspaceID: fixtures.AcmeID,
				ContactID: fixtures.ContactAliceID, Email: fixtures.ContactAliceEmail,
				OutboundMessageID: fixtures.OutboundMessageTrackingSampleID, ObservedAt: observed,
			}))
		}
		for _, action := range []string{systemevent.EmailDelivered, systemevent.EmailBounced, systemevent.EmailComplained} {
			require.NoError(t, env.Bus.Publish(t.Context(), &events.EmailDelivery{
				Action: action, WorkspaceID: fixtures.AcmeID,
				ContactID: fixtures.ContactAliceID, Email: fixtures.ContactAliceEmail,
				OutboundMessageID: fixtures.OutboundMessageTrackingSampleID, ObservedAt: observed,
			}))
		}
	}
	expect := func(at time.Time) {
		t.Helper()
		env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
		msg := env.DB.OutboundMessage.GetX(t.Context(), fixtures.OutboundMessageTrackingSampleID)
		for field, stamp := range map[string]*time.Time{"opened": msg.OpenedAt, "clicked": msg.ClickedAt, "delivered": msg.DeliveredAt, "bounced": msg.BouncedAt, "complained": msg.ComplainedAt} {
			require.NotNil(t, stamp, field)
			assert.True(t, at.Equal(*stamp), "%s retains the earliest accepted observation", field)
		}
	}
	publish(first)
	publish(later)
	before := env.DB.OutboundMessage.GetX(t.Context(), fixtures.OutboundMessageTrackingSampleID)
	assert.Nil(t, before.OpenedAt, "publication leaves the report to its consumer")
	now = later.Add(time.Hour)
	env.PerformEnqueuedJobsNewestFirst(t, jobs.MessageObservationArgs{})
	expect(first)
	env.RetryJobs(t, jobs.MessageObservationArgs{})
	expect(first)
}

func TestAnonymousObservationsSurviveErasureInEitherHandlingOrder(t *testing.T) {
	for name, beforeErasure := range map[string]func(*testing.T, *testhelper.TestEnv){
		"erase-before-handling": func(*testing.T, *testhelper.TestEnv) {},
		"handle-before-erasure": func(t *testing.T, env *testhelper.TestEnv) {
			t.Helper()
			env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
		},
	} {
		t.Run(name, func(t *testing.T) {
			at := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
			env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
			require.NoError(t, env.DB.OutboundMessage.UpdateOneID(fixtures.OutboundMessageErasableTransactionalID).ClearContactID().Exec(t.Context()))
			ids := []int64{fixtures.OutboundMessageErasableBroadcastID, fixtures.OutboundMessageErasableTransactionalID, fixtures.OutboundMessageErasableAutomationID}
			for _, id := range ids {
				msg := env.DB.OutboundMessage.GetX(t.Context(), id)
				contactID := lo.FromPtr(msg.ContactID)
				require.NoError(t, env.Bus.Publish(t.Context(), &events.EmailEngagement{
					Action: systemevent.EmailOpened, WorkspaceID: fixtures.InitechID, ContactID: contactID,
					Email: msg.Destination, OutboundMessageID: id,
				}))
				require.NoError(t, env.Bus.Publish(t.Context(), &events.EmailDelivery{
					Action: systemevent.EmailBounced, WorkspaceID: fixtures.InitechID, ContactID: contactID,
					Email: msg.Destination, OutboundMessageID: id,
				}))
			}
			for _, args := range env.JobsOf(t, jobs.MessageObservationArgs{}.Kind()) {
				encoded, err := json.Marshal(args)
				require.NoError(t, err)
				assert.NotContains(t, string(encoded), fixtures.ContactErasableEmail)
				assert.NotContains(t, string(encoded), "contactId")
			}
			beforeErasure(t, env)
			client := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.InitechID, apitokens.ScopeContactsErase))
			err := client.ContactsDelete(t.Context(), externalapi.ContactsDeleteParams{ID: externalapi.EntityId(fixtures.ContactErasableID)})
			require.NoError(t, err)
			env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
			for _, id := range ids {
				msg := env.DB.OutboundMessage.GetX(t.Context(), id)
				assert.Nil(t, msg.ContactID)
				assert.Empty(t, msg.Destination)
				require.NotNil(t, msg.OpenedAt)
				require.NotNil(t, msg.BouncedAt)
				assert.True(t, at.Equal(*msg.OpenedAt))
				assert.True(t, at.Equal(*msg.BouncedAt))
			}
		})
	}
}

func TestMessageObservationCannotUpdateAnotherWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, env.Bus.Publish(t.Context(), &events.EmailEngagement{
		Action: systemevent.EmailOpened, WorkspaceID: fixtures.GlobexID,
		OutboundMessageID: fixtures.OutboundMessageTrackingSampleID,
	}))
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
	msg := env.DB.OutboundMessage.GetX(t.Context(), fixtures.OutboundMessageTrackingSampleID)
	assert.Nil(t, msg.OpenedAt)
}
