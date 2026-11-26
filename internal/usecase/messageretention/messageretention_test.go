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
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/messageretention"
)

const messagesRetention = 400 * 24 * time.Hour

// past is a clock far enough ahead that every fixture row is older than the window.
func past() time.Time { return time.Now().Add(1000 * 24 * time.Hour) }

func retentionAt(env *testhelper.TestEnv, retention time.Duration, batch int, now time.Time) *messageretention.UseCase {
	return messageretention.New(env.DB, env.Bus, testhelper.Module[*broadcasts.Module](env), retention, batch, func() time.Time { return now })
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
			total.Confirmation += p.Confirmation
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
	VersionSteps                             []automations.VersionStepFigures
}

// reports reads every finished Broadcast's Stats and every Automation's message figures.
func reports(t *testing.T, env *testhelper.TestEnv) (stats map[int64]broadcasts.Stats, figures map[int64]automationFigures) {
	t.Helper()
	ctx := context.Background()
	bs := env.DB.Broadcast.Query().
		Where(broadcast.StatusIn(broadcast.StatusSent, broadcast.StatusFailed, broadcast.StatusCancelled)).
		AllX(ctx)
	stats = map[int64]broadcasts.Stats{}
	for _, b := range bs {
		got, err := testhelper.Module[*broadcasts.Module](env).StatsOf(ctx, env.DB.Scoped(b.WorkspaceID), b.ID)
		require.NoError(t, err)
		stats[b.ID] = got[b.ID]
	}
	m := testhelper.Module[*automations.Module](env)
	figures = map[int64]automationFigures{}
	for _, a := range env.DB.Automation.Query().AllX(ctx) {
		r, err := m.ReportOf(ctx, env.DB.Scoped(a.WorkspaceID), a.ID, automations.ReportPeriod30Days)
		if err != nil {
			continue // archived: no report
		}
		figures[a.ID] = automationFigures{r.Delivered, r.Bounced, r.Complained, r.Captured, r.Steps, r.VersionSteps}
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
		Where(broadcast.StatusIn(broadcast.StatusSent, broadcast.StatusFailed, broadcast.StatusCancelled)).
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

// The Onboarding drip's first email is one Step that both its versions hold (ADR 0122): what its
// pruned messages sent stays under each version and that Step, beside what was pruned before.
func TestPrunedMessagesStayUnderTheirVersionAndStep(t *testing.T) {
	env := testhelper.Setup(t)
	pruneMessages(t, env, messagesRetention, past())

	_, figures := reports(t, env)

	firstEmail := automationstep.ID("2a17428c-014b-5b9e-866e-a08363b2780d")
	drip := figures[fixtures.AutomationOnboardingDripID]
	assert.Equal(t, []automations.VersionStepFigures{
		{Version: 1, StepID: firstEmail, Sent: 4, Skipped: 1},
		{Version: 2, StepID: firstEmail, Skipped: 1},
	}, drip.VersionSteps)
	assert.Equal(t, []automations.StepFigures{{StepID: firstEmail, Sent: 4, Skipped: 2}}, drip.Steps)
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

	require.NoError(t, testhelper.Module[*broadcasts.Delivery](env).SendRecipient(ctx, s, fixtures.BroadcastRecipientUntrackedID))
	assert.Equal(t, pruned, env.DB.OutboundMessage.Query().CountX(ctx))
}

// cancelledFigures is the cancelled fixture Broadcast's figures: both its recipients left unsent.
var cancelledFigures = broadcasts.Stats{Recipients: 2, Cancelled: 2}

// cancelledStats is the report's figures of the cancelled Broadcast.
func cancelledStats(t *testing.T, env *testhelper.TestEnv) broadcasts.Stats {
	t.Helper()
	stats, err := testhelper.Module[*broadcasts.Module](env).StatsOf(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastCancelledID)
	require.NoError(t, err)
	return stats[fixtures.BroadcastCancelledID]
}

// cancelledRows counts the cancelled Broadcast's recipients and says whether its figures are
// frozen.
func cancelledRows(env *testhelper.TestEnv) (recipients int, frozen bool) {
	ctx := context.Background()
	recipients = env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.BroadcastID(fixtures.BroadcastCancelledID)).CountX(ctx)
	frozen = env.DB.BroadcastFigure.Query().Where(broadcastfigure.BroadcastID(fixtures.BroadcastCancelledID)).ExistX(ctx)
	return recipients, frozen
}

// A cancelled Broadcast is final (ADR 0150) and is frozen once its Cancel is past the window (ADR
// 0151): its figures, cancelled included, go to its figures row and its rows are deleted.
func TestPruneMessagesFreezesACancelledBroadcastPastTheWindow(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	before := cancelledStats(t, env)
	require.Equal(t, cancelledFigures, before)
	now := time.Now().Add(messagesRetention)

	pruneMessages(t, env, messagesRetention, now)

	after := cancelledStats(t, env)
	assert.Equal(t, cancelledFigures, after, "the report reads the frozen row, cancelled included")
	figure, err := env.DB.BroadcastFigure.Query().Where(broadcastfigure.BroadcastID(fixtures.BroadcastCancelledID)).Only(ctx)
	require.NoError(t, err, "its figures are frozen")
	assert.Equal(t, 2, figure.Cancelled)
	assert.NotNil(t, figure.PrunedAt)
	recipients, _ := cancelledRows(env)
	assert.Zero(t, recipients)
}

// The window of a cancelled Broadcast counts from its Cancel (two days ago in the fixtures), not
// from when it was created or its recipients were resolved (four and three days ago).
func TestPruneMessagesLeavesACancelledBroadcastInsideTheWindow(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	now := time.Now().Add(messagesRetention - 3*24*time.Hour)
	cutoff := now.Add(-messagesRetention)
	created := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastCancelledID).CreatedAt
	require.Less(t, created, cutoff, "it was created before the window")

	pruneMessages(t, env, messagesRetention, now)

	recipients, frozen := cancelledRows(env)
	assert.False(t, frozen)
	assert.Equal(t, 2, recipients)
	stats := cancelledStats(t, env)
	assert.Equal(t, cancelledFigures, stats)
}

// Umbrella's tracking mix Broadcast (broadcasts.yml) froze with two of its three messages
// untracked (ADR 0120): its figures keep that count, and its opened and clicked over the tracked
// one, once its messages are gone.
func TestAFrozenBroadcastKeepsItsUntrackedCount(t *testing.T) {
	env := testhelper.Setup(t)
	pruneMessages(t, env, messagesRetention, past())

	got, err := testhelper.Module[*broadcasts.Module](env).StatsOf(context.Background(), env.DB.Scoped(fixtures.UmbrellaID), fixtures.BroadcastUmbrellaTrackingMixID)
	require.NoError(t, err)

	mix := got[fixtures.BroadcastUmbrellaTrackingMixID]
	assert.Equal(t, 3, mix.Sent)
	assert.Equal(t, 2, mix.Untracked)
	assert.Equal(t, 1, mix.Opened)
	assert.Equal(t, 1, mix.Clicked)
}
