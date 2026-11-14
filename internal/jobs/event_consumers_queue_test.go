package jobs_test

import (
	"testing"

	"github.com/riverqueue/river"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
)

// Every consumer's job runs in the events queue (ADR 0101), whatever event it is queued for: the
// queue is declared by the job's args, so a consumer whose args forget it would share the default
// queue's workers with everything else.
func TestEveryConsumersJobRunsInTheEventsQueue(t *testing.T) {
	for _, name := range events.Names() {
		env := events.Envelope{ID: "evt", Name: name, WorkspaceID: fixtures.AcmeID, Data: []byte(`{"action":"` + name + `"}`)}
		ev, err := events.Decode(env)
		require.NoError(t, err, name)
		wanting := lo.Filter(jobs.EventConsumers(), func(c events.Consumer, _ int) bool { return c.Wants(env, ev) })
		for _, c := range wanting {
			args := c.Job(env, ev)
			withOpts, ok := args.(river.JobArgsWithInsertOpts)
			require.True(t, ok, "%s: %s declares no InsertOpts", name, args.Kind())
			queue := withOpts.InsertOpts().Queue
			assert.Equal(t, events.QueueEvents, queue, "%s: %s", name, args.Kind())
		}
	}
}
