package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const outboxFloor = 7 * 24 * time.Hour

// pruneEnv publishes n bounce events (one outbox row each) and returns their
// offsets. Inside the test's transaction every row shares one transaction id, so
// the rows differ by offset alone.
func pruneEnv(t *testing.T, n int) (env *testhelper.TestEnv, ids []int64) {
	t.Helper()
	env = testhelper.Setup(t)
	ctx := context.Background()
	for range n {
		require.NoError(t, env.Bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
			return pub.Publish(ctx, &events.EmailDelivery{
				Action: events.NameEmailBounced, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID,
				Email: "prune@example.com", BounceKind: events.BounceKindHard,
			})
		}))
	}
	rows := env.OutboxRows(t)
	require.Len(t, rows, n)
	offsets := make([]int64, n)
	for i, r := range rows {
		offsets[i] = r.Offset
	}
	return env, offsets
}

func setAllCursors(t *testing.T, env *testhelper.TestEnv, txID uint64, acked int64) {
	t.Helper()
	for _, g := range events.ConsumerGroups() {
		env.SetOutboxCursor(t, g, txID, &acked)
	}
}

func prune(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	w := jobs.NewPruneOutboxWorker(env.DB, outboxFloor)
	require.NoError(t, w.Work(context.Background(), &river.Job[jobs.PruneOutboxArgs]{JobRow: &rivertype.JobRow{}}))
}

func remainingOffsets(t *testing.T, env *testhelper.TestEnv) []int64 {
	t.Helper()
	return lo.Map(env.OutboxRows(t), func(r testhelper.OutboxRow, _ int) int64 { return r.Offset })
}

func TestPruneOutboxDeletesRowsBelowLowestCursorPastFloor(t *testing.T) {
	env, off := pruneEnv(t, 4)
	tx := env.OutboxRows(t)[0].TxID
	// Everything but off[1] is old. The cursors sit at off[2], the lowest of them
	// at off[2] too (one group lags behind the rest).
	env.AgeOutbox(t, outboxFloor+time.Hour, off[0], off[2], off[3])
	setAllCursors(t, env, tx, off[3])
	lag := off[2]
	env.SetOutboxCursor(t, events.GroupWebhooks, tx, &lag)

	prune(t, env)

	// off[0]: below the cursor and old -> deleted. off[1]: below but younger than
	// the floor -> kept. off[2]: the cursor row itself -> kept. off[3]: above -> kept.
	assert.Equal(t, off[1:], remainingOffsets(t, env))
}

func TestPruneOutboxSkipsWhenAGroupHasNoOffsetsRow(t *testing.T) {
	env, off := pruneEnv(t, 2)
	tx := env.OutboxRows(t)[0].TxID
	env.AgeOutbox(t, outboxFloor+time.Hour)
	for _, g := range events.ConsumerGroups()[1:] {
		acked := off[1]
		env.SetOutboxCursor(t, g, tx, &acked)
	}

	prune(t, env)

	assert.Equal(t, off, remainingOffsets(t, env), "a group that never consumed may still need every row")
}

func TestPruneOutboxSkipsWhenAGroupHasNullAck(t *testing.T) {
	env, off := pruneEnv(t, 2)
	tx := env.OutboxRows(t)[0].TxID
	env.AgeOutbox(t, outboxFloor+time.Hour)
	setAllCursors(t, env, tx, off[1])
	env.SetOutboxCursor(t, events.GroupPersist, tx, nil)

	prune(t, env)

	assert.Equal(t, off, remainingOffsets(t, env))
}

func TestPruneOutboxRemovesRetiredGroupOffsets(t *testing.T) {
	env, off := pruneEnv(t, 2)
	tx := env.OutboxRows(t)[0].TxID
	env.AgeOutbox(t, outboxFloor+time.Hour)
	setAllCursors(t, env, tx, off[1])
	stale := int64(0)
	env.SetOutboxCursor(t, "retired_group", tx, &stale)

	prune(t, env)

	cursors := env.OutboxCursors(t)
	assert.NotContains(t, cursors, "retired_group")
	assert.Len(t, cursors, len(events.ConsumerGroups()), "registered groups keep their cursor rows")
	assert.Equal(t, off[1:], remainingOffsets(t, env), "the retired group no longer pins the outbox")
}

func TestPruneOutboxRedeliveryDoesNotDuplicateEvents(t *testing.T) {
	env, off := pruneEnv(t, 2)
	ctx := context.Background()
	tx := env.OutboxRows(t)[0].TxID
	env.AgeOutbox(t, outboxFloor+time.Hour)
	setAllCursors(t, env, tx, off[0])
	first := env.OutboxEnvelopes(t)[0]

	require.NoError(t, events.Persist(ctx, env.DB, first))
	prune(t, env)
	// At-least-once: the group whose ack was lost sees the same message again.
	require.NoError(t, events.Persist(ctx, env.DB, first))

	n, err := env.DB.Event.Query().Where(event.SourceID(first.ID)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}
