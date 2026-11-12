package unsubscriberecording_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/unsubscriberecording"
)

func TestAcceptedAttributionSurvivesErasureInEitherHandlingOrder(t *testing.T) {
	for name, beforeErasure := range map[string]func(*testing.T, *testhelper.TestEnv){
		"erase-before-handling": func(*testing.T, *testhelper.TestEnv) {},
		"handle-before-erasure": func(t *testing.T, env *testhelper.TestEnv) {
			t.Helper()
			env.PerformEnqueuedJobs(t, jobs.BroadcastUnsubscribeEventArgs{})
		},
	} {
		t.Run(name, func(t *testing.T) {
			at := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
			env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
			s := env.DB.Scoped(fixtures.InitechID)
			u := unsubscriberecording.New(env.Bus, func() time.Time { return at })
			target := linktarget.Unsubscribe{
				Channel: channels.Email, Source: eligibility.SourceEverything,
				Destination: fixtures.ContactErasableEmail, WorkspaceID: fixtures.InitechID,
				ContactID: fixtures.ContactErasableID, BroadcastID: fixtures.BroadcastInitechID,
				OutboundMessageID: fixtures.OutboundMessageErasableBroadcastID,
			}
			require.NoError(t, u.RecordLink(t.Context(), s, target))
			r := env.DB.BroadcastRecipient.GetX(t.Context(), fixtures.BroadcastRecipientErasableID)
			assert.Nil(t, r.UnsubscribedAt, "the opt-out commits before its report projection")
			for _, args := range env.JobsOf(t, jobs.BroadcastUnsubscribeEventArgs{}.Kind()) {
				encoded, err := json.Marshal(args)
				require.NoError(t, err)
				assert.NotContains(t, string(encoded), fixtures.ContactErasableEmail)
				assert.NotContains(t, string(encoded), "contactId")
			}
			beforeErasure(t, env)
			client := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.InitechID, apitokens.ScopeContactsErase))
			err := client.ContactsDelete(t.Context(), externalapi.ContactsDeleteParams{ID: externalapi.EntityId(fixtures.ContactErasableID)})
			require.NoError(t, err)
			env.PerformEnqueuedJobs(t, jobs.BroadcastUnsubscribeEventArgs{})
			r = env.DB.BroadcastRecipient.GetX(t.Context(), fixtures.BroadcastRecipientErasableID)
			assert.Zero(t, r.ContactID)
			require.NotNil(t, r.UnsubscribedAt)
			assert.True(t, at.Equal(*r.UnsubscribedAt), "the anonymous count retains the accepted fact's time")
			env.RetryJobs(t, jobs.BroadcastUnsubscribeEventArgs{})
			env.PerformEnqueuedJobs(t, jobs.BroadcastUnsubscribeEventArgs{})
			again := env.DB.BroadcastRecipient.GetX(t.Context(), fixtures.BroadcastRecipientErasableID)
			assert.Equal(t, r.UnsubscribedAt, again.UnsubscribedAt)
		})
	}
}

func TestSMSLinkPropagatesMandatorySubscriberConstraintFailures(t *testing.T) {
	env := testhelper.Setup(t)
	failure := &ent.ConstraintError{}
	env.Bus.UseSubscribers(events.Subscriber{
		Wants: func(_ events.Envelope, ev events.DomainEvent) bool { _, ok := ev.(*events.SmsStopObserved); return ok },
		Handle: func(context.Context, *ent.Scoped, events.Publisher, events.Envelope, events.DomainEvent) error {
			return failure
		},
	})
	u := unsubscriberecording.New(env.Bus, time.Now)
	scope := env.DB.Scoped(fixtures.AcmeID)
	err := u.RecordLink(t.Context(), scope, linktarget.Unsubscribe{Channel: channels.SMS, Source: eligibility.SourceEverything, WorkspaceID: fixtures.AcmeID, Destination: fixtures.ContactTexterPhone})
	require.ErrorIs(t, err, failure)
	row, err := unsubscribes.SMSOptOut(t.Context(), scope, fixtures.ContactTexterPhone)
	require.NoError(t, err)
	assert.Nil(t, row)
	assert.Empty(t, env.QueuedEvents(t, events.NameSmsOptOutRecorded))
}
