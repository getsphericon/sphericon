package jobs_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/ingest"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A final delivery report can reach the hook before the send's transaction has recorded the text's
// SID, and Twilio does not post it again (#1044): it is read again shortly instead of being lost.

// lateNotices are the deferred readings of delivery notices on the queue, oldest first.
func lateNotices(t *testing.T, env *testhelper.TestEnv) []ingest.LateNoticeArgs {
	t.Helper()
	rows := env.JobRowsOf(t, ingest.LateNoticeArgs{}.Kind())
	return lo.Map(rows, func(r *rivertype.JobRow, _ int) ingest.LateNoticeArgs {
		var args ingest.LateNoticeArgs
		require.NoError(t, json.Unmarshal(r.EncodedArgs, &args))
		return args
	})
}

// workLate reads the deferred notice again, as its job does.
func workLate(t *testing.T, env *testhelper.TestEnv, args ingest.LateNoticeArgs) {
	t.Helper()
	w := jobs.NewLateNoticeWorker(env.DB, env.Bus, time.Now)
	require.NoError(t, w.Work(t.Context(), &river.Job[ingest.LateNoticeArgs]{Args: args}))
}

func TestADeliveryReportThatComesBeforeItsTextIsCommittedIsReadAgainLater(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.ConnectTwilio(t)
	automationID := smsAutomation(t, env, smsStep(account, "Hello"))
	confirmSMS(t, env, fixtures.ContactTexterPhone)
	runID := enrollVia(t, env, automationID, fixtures.ContactTexterID)
	runSteps(t, env)
	text := textOf(t, env, fixtures.ContactTexterPhone)
	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(ctx)
	// The send's transaction has not recorded the SID yet when the report comes.
	env.DB.OutboundMessage.UpdateOneID(msg.ID).ClearProviderMessageID().ExecX(ctx)
	reportStatus(t, env, account, text, "delivered", "")
	early := env.DB.OutboundMessage.GetX(ctx, msg.ID)
	deferred := lateNotices(t, env)
	require.Len(t, deferred, 1)
	env.DB.OutboundMessage.UpdateOneID(msg.ID).SetProviderMessageID(text.SID).ExecX(ctx)

	workLate(t, env, deferred[0])

	assert.Nil(t, early.DeliveredAt, "nothing matched the report when it came")
	assert.Equal(t, 1, deferred[0].Attempt)
	assert.Equal(t, text.SID, deferred[0].Notice.MessageID)
	assert.Empty(t, deferred[0].Notice.Destination, "the job holds no number")
	settled := env.DB.OutboundMessage.GetX(ctx, msg.ID)
	assert.NotNil(t, settled.DeliveredAt, "the report stamps the text once it is there")
	delivered := env.OutboxCount(t, events.NameSmsDelivered, map[string]any{"contactId": fixtures.ContactTexterID})
	assert.Equal(t, 1, delivered)
}

// A report that never finds its text (one sphericon did not send) is read again a bounded number of
// times, each reading queueing the next, the last queueing none.
func TestADeliveryReportOfNoTextIsReadAgainABoundedNumberOfTimes(t *testing.T) {
	env := testhelper.Setup(t)
	account := env.ConnectTwilio(t)
	automationID := smsAutomation(t, env, smsStep(account, "Hello"))
	confirmSMS(t, env, fixtures.ContactTexterPhone)
	enrollVia(t, env, automationID, fixtures.ContactTexterID)
	runSteps(t, env)
	stranger := textOf(t, env, fixtures.ContactTexterPhone)
	stranger.SID = "SM00000000000000000000000000000000"
	reportStatus(t, env, account, stranger, "delivered", "")
	atTheHook := lateNotices(t, env)
	require.Len(t, atTheHook, 1)
	last := atTheHook[0]
	last.Attempt = len(ingest.LateNoticeDelays)

	workLate(t, env, atTheHook[0])
	workLate(t, env, last)

	queued := lateNotices(t, env)
	attempts := lo.Map(queued, func(a ingest.LateNoticeArgs, _ int) int { return a.Attempt })
	assert.Equal(t, []int{1, 2}, attempts, "the first reading again queues the second; the last queues none")
	delivered := env.OutboxCount(t, events.NameSmsDelivered, nil)
	assert.Zero(t, delivered)
}
