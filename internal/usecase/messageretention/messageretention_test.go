package messageretention_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastfigure"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/messageretention"
)

const messagesRetention = 400 * 24 * time.Hour

// past is a clock far enough ahead that every fixture row is older than the window.
func past() time.Time { return time.Now().Add(1000 * 24 * time.Hour) }

func retentionAt(env *testhelper.TestEnv, retention time.Duration, batch int, now time.Time) *messageretention.UseCase {
	return messageretention.New(env.DB, env.Bus, retention, batch, func() time.Time { return now })
}

// pruneAll runs the prune of every Workspace Due lists, each run within budget, until none says
// more is left, and sums what went.
func pruneAll(t *testing.T, u *messageretention.UseCase, budget int) messageretention.Pruned {
	t.Helper()
	ctx := context.Background()
	due, err := u.Due(ctx)
	require.NoError(t, err)
	var total messageretention.Pruned
	for _, ws := range due {
		for {
			p, err := u.Prune(ctx, ws, budget)
			require.NoError(t, err)
			total.Broadcast += p.Broadcast
			total.Automation += p.Automation
			total.Transactional += p.Transactional
			if !p.More {
				break
			}
		}
	}
	return total
}

func pruneMessages(t *testing.T, env *testhelper.TestEnv, retention time.Duration, now time.Time) messageretention.Pruned {
	t.Helper()
	return pruneAll(t, retentionAt(env, retention, messageretention.Batch, now), messageretention.Budget)
}

// automationFigures is what the Automation report reads from its messages.
type automationFigures struct {
	Delivered, Bounced, Complained, Captured int
	Steps                                    []automations.StepFigures
}

// reports reads every finished Broadcast's Stats and every Automation's message figures.
func reports(t *testing.T, env *testhelper.TestEnv) (stats map[int64]broadcasts.Stats, figures map[int64]automationFigures) {
	t.Helper()
	ctx := context.Background()
	bs := env.DB.Broadcast.Query().
		Where(broadcast.StatusIn(broadcast.StatusSent, broadcast.StatusFailed)).
		AllX(ctx)
	stats = map[int64]broadcasts.Stats{}
	for _, b := range bs {
		got, err := broadcasts.StatsOf(ctx, env.DB.Scoped(b.WorkspaceID), b.ID)
		require.NoError(t, err)
		stats[b.ID] = got[b.ID]
	}
	m := automations.New(nil, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	figures = map[int64]automationFigures{}
	for _, a := range env.DB.Automation.Query().AllX(ctx) {
		r, err := m.ReportOf(ctx, env.DB.Scoped(a.WorkspaceID), a.ID, automations.ReportPeriod30Days)
		if err != nil {
			continue // archived: no report
		}
		figures[a.ID] = automationFigures{r.Delivered, r.Bounced, r.Complained, r.Captured, r.Steps}
	}
	return stats, figures
}

func TestPruneMessagesKeepsEveryReportsFigures(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	beforeStats, beforeFigures := reports(t, env)
	require.NotEmpty(t, beforeStats)
	require.NotEmpty(t, beforeFigures)

	// Batches of one and a budget of three: every loop of the prune turns, and every run stops
	// before its Workspace's work is done and the next one continues from what is left.
	pruned := pruneAll(t, retentionAt(env, messagesRetention, 1, past()), 3)
	assert.Positive(t, pruned.Broadcast)
	assert.Positive(t, pruned.Automation)
	assert.Positive(t, pruned.Transactional)

	afterStats, afterFigures := reports(t, env)
	assert.Equal(t, beforeStats, afterStats, "a frozen Broadcast reports what its rows counted")
	assert.Equal(t, beforeFigures, afterFigures, "an Automation's pruned messages still count")

	finished := env.DB.Broadcast.Query().
		Where(broadcast.StatusIn(broadcast.StatusSent, broadcast.StatusFailed)).
		IDsX(ctx)
	assert.Zero(t, env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.BroadcastIDIn(finished...)).CountX(ctx))
	assert.Zero(t, env.DB.OutboundMessage.Query().Where(outboundmessage.BroadcastIDIn(finished...)).CountX(ctx))
	assert.Zero(t, env.DB.BroadcastFigure.Query().Where(broadcastfigure.PrunedAtIsNil()).CountX(ctx),
		"every frozen Broadcast is marked pruned once its rows are gone")
	assert.Zero(t, env.DB.OutboundMessage.Query().
		Where(
			outboundmessage.KindIn(outboundmessage.KindAutomation, outboundmessage.KindTransactional),
			outboundmessage.StatusNEQ(outboundmessage.StatusPending),
		).
		CountX(ctx))
	assert.Positive(t, env.DB.OutboundMessage.Query().Where(outboundmessage.StatusEQ(outboundmessage.StatusPending)).CountX(ctx),
		"a message still pending is never pruned")
}

func TestPruneMessagesIsIdempotent(t *testing.T) {
	env := testhelper.Setup(t)
	now := past()
	pruneMessages(t, env, messagesRetention, now)
	stats, figures := reports(t, env)
	frozen := frozenRows(t, env)

	assert.Zero(t, pruneMessages(t, env, messagesRetention, now), "nothing is left to delete")

	againStats, againFigures := reports(t, env)
	assert.Equal(t, stats, againStats)
	assert.Equal(t, figures, againFigures)
	assert.Equal(t, frozen, frozenRows(t, env), "a figures row is never recomputed")
}

// frozenRows is every BroadcastFigure as text, its columns and nothing of the client.
func frozenRows(t *testing.T, env *testhelper.TestEnv) []string {
	t.Helper()
	rows := env.DB.BroadcastFigure.Query().Order(broadcastfigure.ByID()).AllX(context.Background())
	out := make([]string, 0, len(rows))
	for _, f := range rows {
		out = append(out, f.String())
	}
	return out
}

func TestPruneMessagesLeavesRowsInsideTheWindow(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	messages := env.DB.OutboundMessage.Query().CountX(ctx)
	frozen := env.DB.BroadcastFigure.Query().CountX(ctx)

	pruneMessages(t, env, 0, past())
	assert.Equal(t, messages, env.DB.OutboundMessage.Query().CountX(ctx), "a zero retention disables the prune")

	pruneMessages(t, env, messagesRetention, time.Now().Add(-messagesRetention))
	assert.Equal(t, messages, env.DB.OutboundMessage.Query().CountX(ctx), "nothing is older than the window")
	assert.Equal(t, frozen, env.DB.BroadcastFigure.Query().CountX(ctx), "no Broadcast is frozen")
}

// A send job of a frozen Broadcast's recipient finds its recipient gone and sends nothing.
func TestAFrozenBroadcastsRecipientSendsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	messages := env.DB.OutboundMessage.Query().CountX(ctx)

	pruneMessages(t, env, messagesRetention, past())
	pruned := env.DB.OutboundMessage.Query().CountX(ctx)
	require.Less(t, pruned, messages)

	require.NoError(t, broadcasts.NewDelivery(env.OutboundModule()).SendRecipient(ctx, s, fixtures.BroadcastRecipientUntrackedID))
	assert.Equal(t, pruned, env.DB.OutboundMessage.Query().CountX(ctx))
}
