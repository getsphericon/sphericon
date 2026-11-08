package events_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/domain/contacts/channelstop"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

type internalObservation struct {
	WorkspaceID int64
	SourceID    string
}

func TestExternalSourceIdentityIsScopedToItsWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	for _, workspaceID := range []int64{fixtures.AcmeID, fixtures.InitechID} {
		fact := &events.CollectedEvent{WorkspaceID: workspaceID, Action: "receipt.scoped", DedupID: "same-upstream-id"}
		require.NoError(t, env.Bus.Publish(t.Context(), fact))
	}
	n := env.DB.Event.Query().Where(event.Action("receipt.scoped")).CountX(t.Context())
	assert.Equal(t, 2, n, "one Workspace must not consume another Workspace's upstream identity")
}

func (*internalObservation) EventName() string  { return "test.internal_observation" }
func (*internalObservation) EventVersion() int  { return 1 }
func (e *internalObservation) Workspace() int64 { return e.WorkspaceID }
func (e *internalObservation) EventID() string  { return e.SourceID }

func TestInternalFactRunsItsSubscriberOnceAndRollsBackFailedConsumption(t *testing.T) {
	env := testhelper.Setup(t)
	boom := errors.New("mandatory reaction failed")
	fail := true
	env.Bus.UseSubscribers(events.Subscriber{
		Wants: func(_ events.Envelope, ev events.DomainEvent) bool {
			_, ok := ev.(*internalObservation)
			return ok
		},
		Handle: func(ctx context.Context, s *ent.Scoped, pub events.Publisher, _ events.Envelope, _ events.DomainEvent) error {
			// test writes: the subscriber's change must commit or roll back with publication
			if err := s.Contact().UpdateOneID(fixtures.ContactAliceID).SetFirstName("Observed").Exec(ctx); err != nil {
				return err
			}
			if err := pub.Publish(ctx, &events.CollectedEvent{WorkspaceID: s.WorkspaceID(), Action: "receipt.handled"}); err != nil {
				return err
			}
			if fail {
				return boom
			}
			return nil
		},
	})
	before := env.DB.Contact.GetX(t.Context(), fixtures.ContactAliceID)
	fact := &internalObservation{WorkspaceID: fixtures.AcmeID, SourceID: "upstream:one"}
	err := env.Bus.Publish(t.Context(), fact)
	require.ErrorIs(t, err, boom)
	after := env.DB.Contact.GetX(t.Context(), fixtures.ContactAliceID)
	assert.Equal(t, before.FirstName, after.FirstName)
	fail = false
	require.NoError(t, env.Bus.Publish(t.Context(), fact))
	after = env.DB.Contact.GetX(t.Context(), fixtures.ContactAliceID)
	assert.Equal(t, "Observed", *after.FirstName)
	require.NoError(t, env.Bus.Publish(t.Context(), fact))
	assert.Equal(t, 1, env.DB.Event.Query().Where(event.Action("receipt.handled")).CountX(t.Context()), "only the successful reaction commits its derived fact")
}

func TestSimultaneousDuplicateSTOPRunsItsReactionOnce(t *testing.T) {
	r := testhelper.NewWorkspaceRace(t, fixtures.AcmeID)
	r.Bus.UseSubscribers(channelstop.Subscribers()...)
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	var wait sync.Once
	r.Bus.UseSubscribers(events.Subscriber{
		Wants: func(_ events.Envelope, ev events.DomainEvent) bool { _, ok := ev.(*events.SmsStopObserved); return ok },
		Handle: func(ctx context.Context, s *ent.Scoped, pub events.Publisher, _ events.Envelope, _ events.DomainEvent) error {
			wait.Do(func() { close(entered); <-release })
			return pub.Publish(ctx, &events.CollectedEvent{WorkspaceID: s.WorkspaceID(), Action: "stop.handled"})
		},
	})
	fact := &events.SmsStopObserved{WorkspaceID: fixtures.AcmeID, Phone: fixtures.ContactTexterPhone, Via: events.UnsubscribedByKeyword, DedupID: "simultaneous-stop"}
	first := make(chan error, 1)
	go func() { first <- r.Bus.Publish(t.Context(), fact) }()
	testhelper.Receive(t, entered, "the first STOP enters its reaction")
	results := testhelper.QueuedBehind(t, r.DB, unblock, testhelper.Writer{Name: "duplicate", Run: func() error { return r.Bus.Publish(t.Context(), fact) }})
	require.NoError(t, testhelper.Receive(t, first, "first publication commits"))
	require.NoError(t, results["duplicate"])
	assert.Equal(t, 1, r.Scope.Event().Query().Where(event.Action("stop.handled")).CountX(t.Context()))
	row, err := unsubscribes.SMSOptOut(t.Context(), r.Scope, fixtures.ContactTexterPhone)
	require.NoError(t, err)
	require.NotNil(t, row, "consent is already stopped without asynchronous work")
}

func TestFailedSMSReactionRollsBackConsentReceiptAndJobsEvenIfCallerIgnoresTheError(t *testing.T) {
	env := testhelper.Setup(t)
	number := fixtures.ContactTexterPhone
	require.NoError(t, env.Bus.Publish(t.Context(), &events.SmsStartObserved{WorkspaceID: fixtures.AcmeID, Phone: number}))
	boom := errors.New("subscriber after consent failed")
	fail := true
	env.Bus.UseSubscribers(events.Subscriber{
		Wants: func(_ events.Envelope, ev events.DomainEvent) bool { _, ok := ev.(*events.SmsStopObserved); return ok },
		Handle: func(context.Context, *ent.Scoped, events.Publisher, events.Envelope, events.DomainEvent) error {
			if fail {
				return boom
			}
			return nil
		},
	})
	fact := &events.SmsStopObserved{WorkspaceID: fixtures.AcmeID, Phone: number, Via: events.UnsubscribedByKeyword, DedupID: "retry-stop"}
	err := env.Bus.WithinTx(t.Context(), func(_ *ent.Client, pub events.Publisher) error {
		require.ErrorIs(t, pub.Publish(t.Context(), fact), boom)
		return nil
	})
	require.ErrorIs(t, err, boom)
	consent, err := confirmations.ReadContactSMS(t.Context(), env.DB.Scoped(fixtures.AcmeID), fixtures.ContactTexterID)
	require.NoError(t, err)
	require.NotNil(t, consent.Confirmation)
	assert.Nil(t, consent.OptOut)
	assert.Zero(t, env.QueuedCount(t, events.NameSmsOptOutRecorded, nil))
	fail = false
	require.NoError(t, env.Bus.Publish(t.Context(), fact))
	consent, err = confirmations.ReadContactSMS(t.Context(), env.DB.Scoped(fixtures.AcmeID), fixtures.ContactTexterID)
	require.NoError(t, err)
	assert.Nil(t, consent.Confirmation)
	require.NotNil(t, consent.OptOut)
	assert.Equal(t, 1, env.QueuedCount(t, events.NameSmsOptOutRecorded, nil))
}

func TestDistinctSemanticFactsMayShareAnExternalSource(t *testing.T) {
	env := testhelper.Setup(t)
	source := "one-external-source"
	require.NoError(t, env.Bus.Publish(t.Context(), &events.SmsStopObserved{WorkspaceID: fixtures.AcmeID, Phone: fixtures.ContactTexterPhone, Via: events.UnsubscribedByKeyword, DedupID: source}))
	require.NoError(t, env.Bus.Publish(t.Context(), &events.SmsStartObserved{WorkspaceID: fixtures.AcmeID, Phone: fixtures.ContactTexterPhone, DedupID: source}))
	consent, err := confirmations.ReadContactSMS(t.Context(), env.DB.Scoped(fixtures.AcmeID), fixtures.ContactTexterID)
	require.NoError(t, err)
	require.NotNil(t, consent.Confirmation)
	assert.Nil(t, consent.OptOut)
}

func TestDistinctProjectedFactsMayShareAnExternalSource(t *testing.T) {
	env := testhelper.Setup(t)
	for _, action := range []string{events.NameEmailDelivered, events.NameEmailBounced} {
		fact := &events.EmailDelivery{Action: action, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Email: fixtures.ContactAliceEmail, OutboundMessageID: fixtures.OutboundMessageTrackingSampleID, DedupID: "shared-notification"}
		require.NoError(t, env.Bus.Publish(t.Context(), fact))
		require.NoError(t, env.Bus.Publish(t.Context(), fact))
	}
	assert.Equal(t, 2, env.DB.Event.Query().Where(event.ActionIn(events.NameEmailDelivered, events.NameEmailBounced), event.SourceIDContains("shared-notification")).CountX(t.Context()))
	assert.Len(t, env.JobsOf(t, jobs.MessageObservationArgs{}.Kind()), 2)
}
