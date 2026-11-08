package events_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/eventjournalentry"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

type failAfterReplayInsert struct {
	events.JobQueue

	failure error
}

func (q failAfterReplayInsert) InsertManyTx(ctx context.Context, tx *sql.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error) {
	if _, err := q.JobQueue.InsertManyTx(ctx, tx, params); err != nil {
		return nil, err
	}
	return nil, q.failure
}

func TestReplayQueueFailureRollsBackEvenAnInsertedPrefix(t *testing.T) {
	at := time.Date(2026, 11, 8, 2, 0, 0, 0, time.UTC)
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
	s := env.DB.Scoped(fixtures.AcmeID)
	require.NoError(t, env.Bus.Publish(t.Context(), &events.EmailEngagement{Action: events.NameEmailOpened, WorkspaceID: fixtures.AcmeID, OutboundMessageID: fixtures.OutboundMessageTrackingSampleID}))
	removeReportJobs(t, env.Bus, s, jobs.MessageObservationArgs{})
	queue, err := river.NewClient(riverdatabasesql.New(env.SQLDB), &river.Config{})
	require.NoError(t, err)
	boom := errors.New("queue failed after inserting")
	env.Bus.UseJobs(failAfterReplayInsert{JobQueue: queue, failure: boom})
	result, err := env.Bus.Replay(t.Context(), fixtures.AcmePublicID, events.ReplayRequest{Consumer: jobs.MessageObservationArgs{}.Kind(), From: at, Before: at.Add(time.Minute)})
	require.ErrorIs(t, err, boom)
	assert.Zero(t, result.Enqueued)
	assert.Empty(t, env.JobsOf(t, jobs.MessageObservationArgs{}.Kind()))
}

// Removing the original jobs simulates River's cleanup, through River's actual delete seam.
func removeReportJobs(t *testing.T, bus *events.Bus, s *ent.Scoped, kinds ...river.JobArgs) {
	t.Helper()
	require.NoError(t, bus.WithinScopedPurgeTx(t.Context(), s, func(_ *ent.Scoped, pub events.PurgingPublisher) error {
		for _, args := range kinds {
			if err := pub.PurgeJobs(t.Context(), jobkind.Key{Kind: args.Kind(), Arg: jobkind.WorkspaceArg}, []int64{s.WorkspaceID()}); err != nil {
				return err
			}
		}
		return nil
	}))
}

func TestReplayReappliesOnlyTheSelectedReportWithItsOriginalObservation(t *testing.T) {
	at := time.Date(2026, 11, 8, 2, 0, 0, 0, time.UTC)
	now := at
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return now }))
	s := env.DB.Scoped(fixtures.AcmeID)
	for _, fact := range []*events.EmailEngagement{
		{Action: events.NameEmailOpened, WorkspaceID: fixtures.AcmeID, OutboundMessageID: fixtures.OutboundMessageTrackingSampleID, ContactID: fixtures.ContactAliceID, Email: fixtures.ContactAliceEmail, ObservedAt: at.Add(-time.Minute)},
		{Action: events.NameEmailOpened, WorkspaceID: fixtures.AcmeID, OutboundMessageID: fixtures.OutboundMessageTrackingSampleID, ObservedAt: at.Add(-2 * time.Minute)},
		{Action: events.NameEmailOpened, WorkspaceID: fixtures.AcmeID, OutboundMessageID: fixtures.OutboundMessageTrackingSampleID, Machine: true},
		{Action: events.NameEmailClicked, WorkspaceID: fixtures.InitechID, OutboundMessageID: fixtures.OutboundMessageErasableBroadcastID},
	} {
		require.NoError(t, env.Bus.Publish(t.Context(), fact))
	}
	// A later recording belongs to neither the requested half-open interval nor its replay.
	now = at.Add(time.Hour)
	require.NoError(t, env.Bus.Publish(t.Context(), &events.EmailEngagement{Action: events.NameEmailClicked, WorkspaceID: fixtures.AcmeID, OutboundMessageID: fixtures.OutboundMessageTrackingSampleID}))
	removeReportJobs(t, env.Bus, s, jobs.MessageObservationArgs{})
	beforeJournal := env.DB.EventJournalEntry.Query().Where(eventjournalentry.WorkspaceID(fixtures.AcmeID)).Order(eventjournalentry.ByID()).IDsX(t.Context())
	beforeWebhooks := len(env.JobsOf(t, jobs.WebhookEventArgs{}.Kind()))
	in := events.ReplayRequest{Consumer: jobs.MessageObservationArgs{}.Kind(), From: at, Before: at.Add(time.Hour), DryRun: true}
	preview, err := env.Bus.Replay(t.Context(), fixtures.AcmePublicID, in)
	require.NoError(t, err)
	assert.Equal(t, events.ReplayResult{Matched: 2}, preview)
	assert.Equal(t, []int64{fixtures.InitechID}, workspacesOfJobs(t, env, jobs.MessageObservationArgs{}.Kind()), "another Workspace's original job stays")
	in.DryRun = false
	for range 2 {
		result, err := env.Bus.Replay(t.Context(), fixtures.AcmePublicID, in)
		require.NoError(t, err)
		assert.Equal(t, events.ReplayResult{Matched: 2, Enqueued: 2}, result)
		env.PerformEnqueuedJobsNewestFirst(t, jobs.MessageObservationArgs{})
	}
	msg := env.DB.OutboundMessage.GetX(t.Context(), fixtures.OutboundMessageTrackingSampleID)
	require.NotNil(t, msg.OpenedAt)
	assert.True(t, at.Add(-2*time.Minute).Equal(*msg.OpenedAt))
	assert.Nil(t, msg.ClickedAt, "the later recording was not replayed")
	assert.Equal(t, beforeJournal, env.DB.EventJournalEntry.Query().Where(eventjournalentry.WorkspaceID(fixtures.AcmeID)).Order(eventjournalentry.ByID()).IDsX(t.Context()), "replay does not republish")
	assert.Len(t, env.JobsOf(t, jobs.WebhookEventArgs{}.Kind()), beforeWebhooks, "no general fan-out")
	assert.NotContains(t, string(env.DB.EventJournalEntry.Query().Where(eventjournalentry.Name(events.NameEmailOpened)).FirstX(t.Context()).Inputs), fixtures.ContactAliceEmail)
}

func TestReplayRefusesUnsupportedConsumersVersionsAndOversizedRangesWithoutPartialJobs(t *testing.T) {
	at := time.Date(2026, 11, 8, 2, 0, 0, 0, time.UTC)
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
	s := env.DB.Scoped(fixtures.AcmeID)
	for range 2 {
		require.NoError(t, env.Bus.Publish(t.Context(), &events.EmailEngagement{Action: events.NameEmailOpened, WorkspaceID: fixtures.AcmeID, OutboundMessageID: fixtures.OutboundMessageTrackingSampleID}))
	}
	removeReportJobs(t, env.Bus, s, jobs.MessageObservationArgs{})
	in := events.ReplayRequest{Consumer: jobs.MessageObservationArgs{}.Kind(), From: at, Before: at.Add(time.Minute), Limit: 1}
	_, err := env.Bus.Replay(t.Context(), fixtures.AcmePublicID, in)
	require.ErrorIs(t, err, events.ErrReplayRangeTooLarge)
	assert.Empty(t, env.JobsOf(t, in.Consumer))
	in.Limit = 2
	in.Consumer = jobs.NoticeEventArgs{}.Kind()
	_, err = env.Bus.Replay(t.Context(), fixtures.AcmePublicID, in)
	require.ErrorIs(t, err, events.ErrReplayConsumerUnsupported)
	in.Consumer = jobs.MessageObservationArgs{}.Kind()

	// The same consumer after an unsupported input-schema change must refuse the retained
	// history, rather than silently decoding it under the new schema. Dry runs validate too.
	bus := events.New(env.SQLDB, entclient.NewTx, func() time.Time { return at })
	consumers := jobs.EventConsumers()
	consumers[0].Replay = events.RetainForReplay[jobs.MessageObservationArgs](2)
	bus.UseConsumers(consumers...)
	in.DryRun = true
	_, err = bus.Replay(t.Context(), fixtures.AcmePublicID, in)
	require.ErrorContains(t, err, "unsupported input version 1")
	assert.Empty(t, env.JobsOf(t, in.Consumer))

	in.From = in.Before
	_, err = env.Bus.Replay(t.Context(), fixtures.AcmePublicID, in)
	require.ErrorContains(t, err, "nonempty recorded-time interval")
	in.From = at
	in.Limit = events.MaxReplayEntries + 1
	_, err = env.Bus.Replay(t.Context(), fixtures.AcmePublicID, in)
	require.ErrorContains(t, err, "replay limit")
}

func TestReplayMeasurementsSurviveContactErasureAndNeverRestoreIdentity(t *testing.T) {
	for name, apply := range map[string]func(erase, replay func()){
		"erasure-before-replay": func(erase, replay func()) { erase(); replay() },
		"replay-before-erasure": func(erase, replay func()) { replay(); erase() },
	} {
		t.Run(name, func(t *testing.T) {
			at := time.Date(2026, 11, 8, 2, 0, 0, 0, time.UTC)
			env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
			s := env.DB.Scoped(fixtures.InitechID)
			require.NoError(t, env.Bus.Publish(t.Context(), &events.EmailEngagement{Action: events.NameEmailOpened, WorkspaceID: fixtures.InitechID, ContactID: fixtures.ContactErasableID, Email: fixtures.ContactErasableEmail, OutboundMessageID: fixtures.OutboundMessageErasableBroadcastID}))
			require.NoError(t, env.Bus.Publish(t.Context(), &events.MessageUnsubscribed{WorkspaceID: fixtures.InitechID, OutboundMessageID: fixtures.OutboundMessageErasableBroadcastID, ObservedAt: at}))
			removeReportJobs(t, env.Bus, s, jobs.MessageObservationArgs{}, jobs.BroadcastUnsubscribeEventArgs{})
			erase := func() {
				client := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.InitechID, "contacts:erase"))
				require.NoError(t, client.ContactsDelete(t.Context(), externalapi.ContactsDeleteParams{ID: externalapi.EntityId(fixtures.ContactErasableID)}))
			}
			replay := func() {
				for range 2 {
					for _, args := range []river.JobArgs{jobs.MessageObservationArgs{}, jobs.BroadcastUnsubscribeEventArgs{}} {
						result, err := env.Bus.Replay(t.Context(), fixtures.InitechPublicID, events.ReplayRequest{Consumer: args.Kind(), From: at, Before: at.Add(time.Minute)})
						require.NoError(t, err)
						assert.Equal(t, 1, result.Enqueued)
						env.PerformEnqueuedJobs(t, args)
					}
				}
			}
			apply(erase, replay)
			msg := env.DB.OutboundMessage.GetX(t.Context(), fixtures.OutboundMessageErasableBroadcastID)
			assert.Nil(t, msg.ContactID)
			assert.Empty(t, msg.Destination)
			require.NotNil(t, msg.OpenedAt)
			assert.True(t, at.Equal(*msg.OpenedAt))
			recipient := env.DB.BroadcastRecipient.GetX(t.Context(), fixtures.BroadcastRecipientErasableID)
			assert.Zero(t, recipient.ContactID)
			require.NotNil(t, recipient.UnsubscribedAt)
			assert.True(t, at.Equal(*recipient.UnsubscribedAt))
		})
	}
}

func TestReplayAndJournalRespectWorkspacePurge(t *testing.T) {
	at := time.Date(2026, 11, 8, 2, 0, 0, 0, time.UTC)
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
	for _, ws := range []int64{fixtures.AcmeID, fixtures.InitechID} {
		require.NoError(t, env.Bus.Publish(t.Context(), &events.MessageUnsubscribed{WorkspaceID: ws, OutboundMessageID: fixtures.OutboundMessageTrackingSampleID, ObservedAt: at}))
	}
	require.NoError(t, env.Bus.Publish(t.Context(), &events.PasskeyChange{Action: events.NameUserPasskeyAdded, Email: "private@example.com"}))
	require.NoError(t, env.Bus.WithinWorkspacePurgeTx(t.Context(), func(_ *ent.Client, purger events.WorkspacePurger) error {
		return purger.PurgeWorkspaceJobs(t.Context(), fixtures.InitechID)
	}))
	assert.Zero(t, env.DB.EventJournalEntry.Query().Where(eventjournalentry.WorkspaceID(fixtures.InitechID)).CountX(t.Context()))
	assert.Equal(t, 1, env.DB.EventJournalEntry.Query().Where(eventjournalentry.WorkspaceID(fixtures.AcmeID)).CountX(t.Context()))
	instanceCount := env.DB.EventJournalEntry.Query().Where(eventjournalentry.WorkspaceIDIsNil(), eventjournalentry.Name(events.NameUserPasskeyAdded)).CountX(t.Context())
	assert.Equal(t, 1, instanceCount)
	result, err := env.Bus.Replay(t.Context(), fixtures.InitechPublicID, events.ReplayRequest{Consumer: jobs.BroadcastUnsubscribeEventArgs{}.Kind(), From: at, Before: at.Add(time.Minute)})
	require.NoError(t, err)
	assert.Zero(t, result.Matched)
	_, err = env.Bus.Replay(t.Context(), "NoSuchId", events.ReplayRequest{Consumer: jobs.BroadcastUnsubscribeEventArgs{}.Kind(), From: at, Before: at.Add(time.Minute)})
	require.ErrorIs(t, err, events.ErrReplayWorkspaceUnavailable)
}

func TestReplayBroadcastAttributionConvergesAcrossDistinctFactsInEitherOrder(t *testing.T) {
	for name, perform := range map[string]func(*testing.T, *testhelper.TestEnv){
		"oldest-first": func(t *testing.T, env *testhelper.TestEnv) {
			t.Helper()
			env.PerformEnqueuedJobs(t, jobs.BroadcastUnsubscribeEventArgs{})
		},
		"newest-first": func(t *testing.T, env *testhelper.TestEnv) {
			t.Helper()
			env.PerformEnqueuedJobsNewestFirst(t, jobs.BroadcastUnsubscribeEventArgs{})
		},
	} {
		t.Run(name, func(t *testing.T) {
			at := time.Date(2026, 11, 8, 2, 0, 0, 0, time.UTC)
			env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
			s := env.DB.Scoped(fixtures.InitechID)
			for _, observed := range []time.Time{at.Add(-time.Minute), at} {
				require.NoError(t, env.Bus.Publish(t.Context(), &events.MessageUnsubscribed{WorkspaceID: fixtures.InitechID, OutboundMessageID: fixtures.OutboundMessageErasableBroadcastID, ObservedAt: observed}))
			}
			removeReportJobs(t, env.Bus, s, jobs.BroadcastUnsubscribeEventArgs{})
			for range 2 {
				result, err := env.Bus.Replay(t.Context(), fixtures.InitechPublicID, events.ReplayRequest{Consumer: jobs.BroadcastUnsubscribeEventArgs{}.Kind(), From: at, Before: at.Add(time.Minute)})
				require.NoError(t, err)
				assert.Equal(t, 2, result.Enqueued)
				perform(t, env)
				recipient := env.DB.BroadcastRecipient.GetX(t.Context(), fixtures.BroadcastRecipientErasableID)
				require.NotNil(t, recipient.UnsubscribedAt)
				assert.True(t, at.Add(-time.Minute).Equal(*recipient.UnsubscribedAt), "attribution keeps the earliest accepted observation")
			}
		})
	}
}
