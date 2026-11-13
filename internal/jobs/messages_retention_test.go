package jobs_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The daily tick queues a prune for each Workspace with something past the window, and each
// prune deletes its Workspace's expired messages (ADR 0143, ADR 0144).
func TestTheMessageRetentionTickFansOutAPrunePerWorkspace(t *testing.T) {
	later := time.Now().Add(1000 * 24 * time.Hour)
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return later }))
	ctx := t.Context()

	env.EnqueueJob(t, jobs.PruneMessagesArgs{})
	env.PerformEnqueuedJobs(t, jobs.PruneMessagesArgs{})
	queued := env.JobsOf(t, jobs.PruneWorkspaceMessagesArgs{}.Kind())
	assert.Greater(t, len(queued), 1, "one prune per Workspace with expired messages")

	env.PerformEnqueuedJobs(t, jobs.PruneWorkspaceMessagesArgs{})
	assert.Zero(t, env.DB.OutboundMessage.Query().
		Where(
			outboundmessage.KindIn(outboundmessage.KindAutomation, outboundmessage.KindTransactional),
			outboundmessage.StatusNEQ(outboundmessage.StatusPending),
		).
		CountX(ctx), "every Automation and Transactional message that left pending is past the window and gone")
}
