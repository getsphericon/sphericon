package workspacedeletion_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/riverqueue/river/rivertype"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/domain/insights/workspaceoversight"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/hook"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/trackinghostchange"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/workspacedeletion"
)

// deleteUmbrella puts Umbrella into its deletion window, deleted ago before now.
func deleteUmbrella(t *testing.T, env *testhelper.TestEnv, ago time.Duration) {
	t.Helper()
	env.FeedDB.Workspace.UpdateOneID(fixtures.UmbrellaID).SetDiscardedAt(now.Add(-ago)).ExecX(t.Context())
}

// runErase is one erase at now.
func runErase(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	m := workspacedeletion.New(env.DB, env.Bus, func() time.Time { return now })
	_, err := m.EraseDue(context.Background())
	require.NoError(t, err)
}

func rowsOf(t *testing.T, env *testhelper.TestEnv, ws int64) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, e := range ent.ScopedEntities() {
		ids, err := e.IDs(t.Context(), env.DB, ws)
		require.NoError(t, err)
		counts[e.Name] = len(ids)
	}
	return counts
}

// rowsOfAll is the rows of every entity across every Workspace of the fixtures.
func rowsOfAll(t *testing.T, env *testhelper.TestEnv) map[string]int {
	t.Helper()
	ids := env.DB.Workspace.Query().IDsX(t.Context())
	counts := map[string]int{}
	for _, ws := range ids {
		for name, n := range rowsOf(t, env, ws) {
			counts[name] += n
		}
	}
	return counts
}

// Every Workspace of the fixtures is erased, so an entity with no step leaves its rows behind.
// The fixtures hold rows of every scoped entity, which the first check requires.
func TestEraseCoversEveryWorkspaceOwnedEntity(t *testing.T) {
	env := testhelper.Setup(t)
	env.FeedDB.Workspace.Update().SetDiscardedAt(now.Add(-workspaces.DeletionWindow - time.Hour)).ExecX(t.Context())
	before := rowsOfAll(t, env)
	for _, e := range ent.ScopedEntities() {
		assert.Positivef(t, before[e.Name], "the fixtures hold a %s row", e.Name)
	}

	runErase(t, env)

	for name, n := range rowsOfAll(t, env) {
		assert.Zerof(t, n, "%s rows are erased", name)
	}
}

// writeEventAfterContactDelete is an ent hook that, after a Contact is deleted, writes an Event of
// Umbrella: a row written while the erase runs.
func writeEventAfterContactDelete(next ent.Mutator) ent.Mutator {
	return hook.ContactFunc(func(ctx context.Context, m *ent.ContactMutation) (ent.Value, error) {
		v, err := next.Mutate(ctx, m)
		if err != nil || !m.Op().Is(ent.OpDelete|ent.OpDeleteOne) {
			return v, err
		}
		// test writes: a concurrent write simulated inside a hook is the race under test
		return v, m.Client().Event.Create().SetWorkspaceID(fixtures.UmbrellaID).SetAction("written.during.erasure").Exec(ctx)
	})
}

func TestEraseDueLeavesAWorkspaceInsideItsWindow(t *testing.T) {
	env := testhelper.Setup(t)
	deleteUmbrella(t, env, workspaces.DeletionWindow-time.Hour)
	before := rowsOf(t, env, fixtures.UmbrellaID)

	runErase(t, env)

	after := rowsOf(t, env, fixtures.UmbrellaID)
	assert.Equal(t, before, after)
	ws := env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID)
	assert.Nil(t, ws.ErasedAt)
	assert.NotEmpty(t, ws.Name)
}

func TestEraseDueRemovesWhatAWorkspaceOwnedAndKeepsATombstone(t *testing.T) {
	env := testhelper.Setup(t)
	deleteUmbrella(t, env, workspaces.DeletionWindow+time.Hour)
	before := env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID)
	acme := rowsOf(t, env, fixtures.AcmeID)
	require.NotZero(t, rowsOf(t, env, fixtures.UmbrellaID)["SendingDomain"])
	require.NotZero(t, rowsOf(t, env, fixtures.UmbrellaID)["CapturedMessage"], "Umbrella's Test inbox holds a Captured message")

	runErase(t, env)

	for name, n := range rowsOf(t, env, fixtures.UmbrellaID) {
		assert.Zerof(t, n, "%s rows are erased", name)
	}
	acmeAfter := rowsOf(t, env, fixtures.AcmeID)
	assert.Equal(t, acme, acmeAfter, "another Workspace is untouched")
	ws := env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID)
	assert.Equal(t, before.PublicID, ws.PublicID)
	assert.True(t, before.CreatedAt.Equal(ws.CreatedAt))
	assert.True(t, before.DiscardedAt.Equal(*ws.DiscardedAt))
	assert.True(t, now.Equal(*ws.ErasedAt))
	assert.Empty(t, ws.Name)
	assert.NotEqual(t, before.CollectKey, ws.CollectKey, "the collect key no longer opens anything")
}

func TestEraseDueReleasesTheTrackingHostsAndAdvancesTheFeed(t *testing.T) {
	env := testhelper.Setup(t)
	deleteUmbrella(t, env, workspaces.DeletionWindow+time.Hour)
	_, cursor := feedTail(t, env, "")

	runErase(t, env)

	removed := env.DB.TrackingHostChange.Query().
		Where(trackinghostchange.WorkspaceID(fixtures.UmbrellaID), trackinghostchange.StateEQ(trackinghostchange.StateRemoved)).
		AllX(t.Context())
	hosts := lo.Map(removed, func(c *ent.TrackingHostChange, _ int) string { return c.Host })
	assert.ElementsMatch(t, []string{fixtures.TrackingDomainServingHost, fixtures.TrackingDomainRetiredHost}, hosts)

	entries, _ := feedTail(t, env, cursor)
	require.Len(t, entries, 1)
	tomb := entries[0]
	assert.Equal(t, int64(fixtures.UmbrellaID), tomb.Workspace.ID)
	assert.NotNil(t, tomb.Workspace.ErasedAt)
	assert.NotNil(t, tomb.Workspace.DiscardedAt)
	assert.Empty(t, tomb.Workspace.Name)
	assert.Empty(t, tomb.Owners)
}

// Deleting a row can write another: here deleting a Contact writes an Event, after the
// Events step has run. The erasure settles, so nothing is left behind the tombstone.
func TestEraseDueRemovesRowsWrittenWhileItDeletes(t *testing.T) {
	env := testhelper.Setup(t)
	deleteUmbrella(t, env, workspaces.DeletionWindow+time.Hour)
	env.DB.Contact.Use(writeEventAfterContactDelete)

	runErase(t, env)

	for name, n := range rowsOf(t, env, fixtures.UmbrellaID) {
		assert.Zerof(t, n, "%s rows are erased", name)
	}
	ws := env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID)
	assert.NotNil(t, ws.ErasedAt)
}

// Initech's sent Broadcast is referenced by Outbound messages (RESTRICT), so the erase removes the
// messages before the Broadcast; the other Workspaces keep theirs.
func TestEraseRemovesABroadcastThatHasSentOutboundMessages(t *testing.T) {
	env := testhelper.Setup(t)
	env.FeedDB.Workspace.UpdateOneID(fixtures.InitechID).SetDiscardedAt(now.Add(-workspaces.DeletionWindow - time.Hour)).ExecX(t.Context())
	sent := env.DB.Scoped(fixtures.InitechID).OutboundMessage().Query().Where(outboundmessage.BroadcastID(fixtures.BroadcastInitechID)).CountX(t.Context())
	require.NotZero(t, sent)
	acmeBefore := rowsOf(t, env, fixtures.AcmeID)

	runErase(t, env)

	initech := rowsOf(t, env, fixtures.InitechID)
	acmeAfter := rowsOf(t, env, fixtures.AcmeID)
	assert.Zero(t, initech["Broadcast"])
	assert.Zero(t, initech["OutboundMessage"])
	assert.Equal(t, acmeBefore, acmeAfter)
}

func TestASecondEraseRunChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	deleteUmbrella(t, env, workspaces.DeletionWindow+time.Hour)
	runErase(t, env)
	_, cursor := feedTail(t, env, "")
	hostLog := env.DB.TrackingHostChange.Query().CountX(t.Context())
	first := env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID)

	runErase(t, env)

	entries, _ := feedTail(t, env, cursor)
	hostLogAfter := env.DB.TrackingHostChange.Query().CountX(t.Context())
	second := env.DB.Workspace.GetX(t.Context(), fixtures.UmbrellaID)
	assert.Empty(t, entries, "the feed does not move")
	assert.Equal(t, hostLog, hostLogAfter)
	assert.True(t, first.ErasedAt.Equal(*second.ErasedAt))
}

// feedTail reads the Workspace feed after cursor and returns its entries and the next cursor.
func feedTail(t *testing.T, env *testhelper.TestEnv, cursor string) (entries []workspaceoversight.FeedEntry, next string) {
	t.Helper()
	page, err := env.StaffEnterprise.WorkspaceFeed(t.Context(), cursor, 200)
	require.NoError(t, err)
	return page.Items, page.NextCursor
}

// bounceIn publishes a hard bounce of address in the Workspace through the bus, which queues its
// consumers' jobs (the suppression, the webhooks fan-out) as any producer's publish does.
func bounceIn(t *testing.T, env *testhelper.TestEnv, workspaceID int64, address string) {
	t.Helper()
	require.NoError(t, env.Bus.WithinTx(t.Context(), func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(t.Context(), &events.EmailDelivery{
			Action: events.NameEmailBounced, WorkspaceID: workspaceID, Email: address,
			BounceKind: events.BounceKindHard, DedupID: fmt.Sprintf("bounce-%d", workspaceID),
		})
	}))
}

// workspacesOfJobs is the Workspace each queued job of kind names, oldest first.
func workspacesOfJobs(t *testing.T, env *testhelper.TestEnv, kind string) []int64 {
	t.Helper()
	return lo.Map(env.JobsOf(t, kind), func(args map[string]any, _ int) int64 {
		id, _ := args[jobkind.WorkspaceArg].(float64)
		return int64(id)
	})
}

// The queue keeps a job's args, which carry personal data (an event's address, a webhook's body),
// for days after it ran, and a waiting job still runs: the erase removes every job of the
// Workspace with its rows, waiting, scheduled and discarded alike. A running one is left to
// finish, and another Workspace's jobs and the Instance's stay.
func TestEraseDueRemovesTheWorkspacesJobs(t *testing.T) {
	env := testhelper.Setup(t)
	deleteUmbrella(t, env, workspaces.DeletionWindow+time.Hour)
	later := now.Add(7 * 24 * time.Hour)
	for _, ws := range []int64{fixtures.UmbrellaID, fixtures.AcmeID} {
		bounceIn(t, env, ws, "bounced@example.com")
		env.EnqueueDiscardedJob(t, jobs.DeliverWebhookArgs{WorkspaceID: ws, EndpointID: 1, EventName: "email.bounced", DeliveryID: "d", Body: []byte(`{"data":{"email":"bounced@example.com"}}`)})
		env.EnqueueScheduledJob(t, broadcasts.SendArgs{WorkspaceID: ws, BroadcastID: 1}, later)
	}
	env.EnqueueRunningJob(t, automations.RunStepArgs{WorkspaceID: fixtures.UmbrellaID, RunID: 1})
	env.EnqueueJob(t, jobs.PruneEventsArgs{})
	require.Equal(t, []int64{fixtures.UmbrellaID, fixtures.AcmeID}, workspacesOfJobs(t, env, jobs.SuppressEventArgs{}.Kind()), "the publish queued the suppression")

	runErase(t, env)

	suppress := workspacesOfJobs(t, env, jobs.SuppressEventArgs{}.Kind())
	fanOuts := workspacesOfJobs(t, env, jobs.WebhookEventArgs{}.Kind())
	deliveries := workspacesOfJobs(t, env, jobs.DeliverWebhookArgs{}.Kind())
	sends := workspacesOfJobs(t, env, broadcasts.SendArgs{}.Kind())
	steps := workspacesOfJobs(t, env, automations.RunStepArgs{}.Kind())
	prunes := env.JobsOf(t, jobs.PruneEventsArgs{}.Kind())
	assert.Equal(t, []int64{fixtures.AcmeID}, suppress)
	assert.Equal(t, []int64{fixtures.AcmeID}, fanOuts)
	assert.Equal(t, []int64{fixtures.AcmeID}, deliveries, "a discarded delivery goes")
	assert.Equal(t, []int64{fixtures.AcmeID}, sends, "a scheduled send goes")
	assert.Equal(t, []int64{fixtures.UmbrellaID}, steps, "a running job is left to finish")
	assert.Len(t, prunes, 1, "a job of the Instance stays")
}

// lateBounce is a suppression consumer's job for a hard bounce of address in the Workspace, as the
// bus queued it before the erase: a job a worker fetched before the purge and runs again after it.
func lateBounce(t *testing.T, workspaceID int64, address string) jobs.SuppressEventArgs {
	t.Helper()
	data, err := json.Marshal(&events.EmailDelivery{
		Action: events.NameEmailBounced, WorkspaceID: workspaceID, Email: address, BounceKind: events.BounceKindHard,
	})
	require.NoError(t, err)
	return jobs.SuppressEventArgs{
		ID: fmt.Sprintf("late-%d", workspaceID), Name: events.NameEmailBounced, Version: 1, WorkspaceID: workspaceID, Data: data,
	}
}

// An event consumer writes into its envelope's Workspace without loading a row of it: one that
// runs after the erase is cancelled before it writes, so nothing lands behind the tombstone,
// while the same job of a live Workspace still writes.
func TestAJobOfAnErasedWorkspaceWritesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	deleteUmbrella(t, env, workspaces.DeletionWindow+time.Hour)
	runErase(t, env)
	env.EnqueueJob(t, lateBounce(t, fixtures.UmbrellaID, "late@example.com"))
	env.EnqueueJob(t, lateBounce(t, fixtures.AcmeID, "late@example.com"))

	env.PerformEnqueuedJobs(t, jobs.SuppressEventArgs{})

	umbrella := rowsOf(t, env, fixtures.UmbrellaID)
	acme := env.DB.Suppression.Query().Where(suppression.WorkspaceID(fixtures.AcmeID), suppression.Destination("late@example.com")).CountX(t.Context())
	states := lo.Map(env.JobRowsOf(t, jobs.SuppressEventArgs{}.Kind()), func(j *rivertype.JobRow, _ int) rivertype.JobState { return j.State })
	assert.Zero(t, umbrella["Suppression"], "nothing is written into the erased Workspace")
	assert.Equal(t, 1, acme, "the live Workspace's job still writes")
	assert.Equal(t, []rivertype.JobState{rivertype.JobStateCancelled, rivertype.JobStateCompleted}, states)
}
