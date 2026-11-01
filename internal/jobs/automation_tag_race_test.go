package jobs_test

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/contactlock"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// writer is one side of a race over a Contact: a transaction the test starts and names.
type writer struct {
	name string
	run  func() error
}

// queuedOnTheContact runs the writers in their order while a transaction of the test's own holds
// the Contact's row, each started once the ones before it wait for a lock, and then releases the
// row: the writers meet in the order they queued, when all of them are already inside their
// transactions. It returns each writer's error by its name.
func queuedOnTheContact(t *testing.T, e *realEnv, contactID int64, writers ...writer) map[string]error {
	t.Helper()
	ctx := context.Background()
	tx, err := e.client.Tx(ctx)
	require.NoError(t, err)
	release := sync.OnceFunc(func() { _ = tx.Rollback() })
	t.Cleanup(release) // a failed assertion must not leave the Contact locked
	_, err = contactlock.Lock(ctx, e.scoped.InTx(tx.Client(), nil), contactID)
	require.NoError(t, err)
	probe, err := sql.Open("pgx", e.url)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(probe, "probe") })

	results := make(map[string]chan error, len(writers))
	for i, w := range writers {
		result, done := make(chan error, 1), make(chan struct{})
		results[w.name] = result
		go func() {
			result <- w.run()
			close(done)
		}()
		require.Eventually(t, func() bool {
			waiting, err := testhelper.LockWaiters(ctx, probe)
			return settled(done) || (err == nil && waiting > i)
		}, 10*time.Second, 5*time.Millisecond, "%s finished or waits for a lock", w.name)
	}
	release()
	errs := make(map[string]error, len(results))
	for name, result := range results {
		errs[name] = <-result
	}
	return errs
}

// A tag step and an exit Event of the same Contact both take the Contact's lock before they touch
// the Enrollment, so neither waits for the other in a cycle: both commit and the Enrollment exits.
// The Tag is applied when the step moved the Enrollment before the exit ended it, and never after
// the exit (#811).
func TestATagStepAndAnExitOfOneContactNeverDeadlock(t *testing.T) {
	for _, tc := range []struct {
		name       string
		queue      func(step, exit writer) []writer
		wantStep   int
		wantTagged bool
	}{
		{
			name:     "the exit queues first and wins",
			queue:    func(step, exit writer) []writer { return []writer{exit, step} },
			wantStep: 0, wantTagged: false,
		},
		{
			name:     "the step queues first and tags",
			queue:    func(step, exit writer) []writer { return []writer{step, exit} },
			wantStep: 1, wantTagged: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newRealEnv(t)
			ctx := context.Background()
			runID, enrolled, err := automations.Enroll(ctx, e.scoped, testhelper.PublisherOn(e.bus(t)), fixtures.AutomationJobsTagWithExitID, fixtures.ContactAliceID)
			require.NoError(t, err)
			require.True(t, enrolled)
			stepper := e.stepper(t, &fakeSender{})
			ending := e.bus(t)
			step := writer{name: "step", run: func() error { return stepper.Step(ctx, e.scoped, runID, 0) }}
			exit := writer{name: "exit", run: func() error {
				left := automations.Event{Action: "job_tag_exit_left", OccurredAt: time.Now()}
				return stepping.OnEvent(ctx, ending, e.scoped, fixtures.ContactAliceID, left, 0)
			}}

			errs := queuedOnTheContact(t, e, fixtures.ContactAliceID, tc.queue(step, exit)...)
			require.Equal(t, map[string]error{"step": nil, "exit": nil}, errs, "neither side is aborted")

			run, err := e.client.AutomationRun.Get(ctx, runID)
			require.NoError(t, err)
			tagged, err := e.client.Contact.Query().
				Where(contact.ID(fixtures.ContactAliceID)).
				QueryTags().Where(tag.Name("tag-race")).
				Exist(ctx)
			require.NoError(t, err)

			assert.Equal(t, automationrun.StatusExited, run.Status)
			assert.Equal(t, tc.wantStep, run.CurrentStep)
			assert.Equal(t, tc.wantTagged, tagged)
		})
	}
}
