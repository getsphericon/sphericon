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
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactlock"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/contacterasure"
)

// queuedOnTheContact runs the writers queued behind a transaction of the test's own that holds the
// Contact's row (testhelper.QueuedBehind), then releases the row. It returns each testhelper.Writer's error by
// its name.
func queuedOnTheContact(t *testing.T, e *realEnv, contactID int64, writers ...testhelper.Writer) map[string]error {
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
	return testhelper.QueuedBehind(t, probe, release, writers...)
}

// enrolledForTheExit enrolls Alice in the Automation whose tag step an exit Event ends, and returns
// the Enrollment and the writer of that exit Event.
func enrolledForTheExit(t *testing.T, e *realEnv) (int64, testhelper.Writer) {
	t.Helper()
	ctx := context.Background()
	runID := e.enrolledNow(t, fixtures.AutomationJobsTagWithExitID, fixtures.ContactAliceID)
	ending := e.bus(t)
	return runID, testhelper.Writer{Name: "exit", Run: func() error {
		left := automations.Event{Action: "job_tag_exit_left", OccurredAt: time.Now()}
		return e.enrollments(ending).OnEvent(ctx, e.scoped, automations.OnEventInput{ContactID: fixtures.ContactAliceID, Event: left})
	}}
}

// A tag step and an exit Event of the same Contact both take the Contact's lock before they touch
// the Enrollment, so neither waits for the other in a cycle: both commit and the Enrollment exits.
// The Tag is applied when the step moved the Enrollment before the exit ended it, and never after
// the exit (#811).
func TestATagStepAndAnExitOfOneContactNeverDeadlock(t *testing.T) {
	for _, tc := range []struct {
		name       string
		queue      func(step, exit testhelper.Writer) []testhelper.Writer
		wantStep   int
		wantTagged bool
	}{
		{
			name:     "the exit queues first and wins",
			queue:    func(step, exit testhelper.Writer) []testhelper.Writer { return []testhelper.Writer{exit, step} },
			wantStep: 0, wantTagged: false,
		},
		{
			name:     "the step queues first and tags",
			queue:    func(step, exit testhelper.Writer) []testhelper.Writer { return []testhelper.Writer{step, exit} },
			wantStep: 1, wantTagged: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newRealEnv(t)
			ctx := context.Background()
			runID, exit := enrolledForTheExit(t, e)
			stepper := e.stepper(t, &fakeSender{})
			step := testhelper.Writer{Name: "step", Run: func() error { return stepper.Step(ctx, e.scoped, runID, 0) }}

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

// An Erasure deletes the Contact's Enrollments and then the Contact. It locks the Contact first, as
// an exit Event does, so the two never wait for each other in a cycle: whichever queued first runs
// whole, an exit after the Erasure finds no Contact left, and the Contact and its Enrollment are
// gone at the end (#811).
func TestAnErasureAndAnExitOfOneContactNeverDeadlock(t *testing.T) {
	for _, tc := range []struct {
		name  string
		queue func(erase, exit testhelper.Writer) []testhelper.Writer
	}{
		{name: "the exit queues first", queue: func(erase, exit testhelper.Writer) []testhelper.Writer { return []testhelper.Writer{exit, erase} }},
		{name: "the erasure queues first", queue: func(erase, exit testhelper.Writer) []testhelper.Writer { return []testhelper.Writer{erase, exit} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newRealEnv(t)
			ctx := context.Background()
			runID, exit := enrolledForTheExit(t, e)
			erasing := contacterasure.New(e.bus(t), time.Now)
			erase := testhelper.Writer{Name: "erase", Run: func() error {
				return erasing.Erase(ctx, e.scoped, contacterasure.Identifier{ContactID: fixtures.ContactAliceID}, contacterasure.Requester{Kind: contacterasure.RequesterAPIToken})
			}}

			errs := queuedOnTheContact(t, e, fixtures.ContactAliceID, tc.queue(erase, exit)...)
			require.Equal(t, map[string]error{"erase": nil, "exit": nil}, errs, "neither side is aborted")

			contactLeft, err := e.client.Contact.Query().Where(contact.ID(fixtures.ContactAliceID)).Exist(ctx)
			require.NoError(t, err)
			runLeft, err := e.client.AutomationRun.Query().Where(automationrun.ID(runID)).Exist(ctx)
			require.NoError(t, err)

			assert.False(t, contactLeft, "the Contact is erased")
			assert.False(t, runLeft, "its Enrollment is erased with it")
		})
	}
}
