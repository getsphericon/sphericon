package testhelper_test

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/postgres"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// failures is a test the harness is run under to see it fail: it records what the harness
// reports and ends the goroutine on a fatal failure, as the testing package does.
type failures struct {
	testing.TB

	mu   sync.Mutex
	said []string
}

func (f *failures) Helper() {}

func (f *failures) Errorf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.said = append(f.said, fmt.Sprintf(format, args...))
}

func (f *failures) Fatalf(format string, args ...any) {
	f.Errorf(format, args...)
	runtime.Goexit()
}

func (f *failures) FailNow() { runtime.Goexit() }

// report is everything the harness reported, once.
func (f *failures) report() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.said, "\n")
}

// failing runs harness under a failures of t and returns what it reported; it fails t when the
// harness reported nothing.
func failing(t *testing.T, harness func(tb testing.TB)) string {
	t.Helper()
	f := &failures{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		harness(f)
	}()
	<-done
	report := f.report()
	require.NotEmpty(t, report, "the harness let it pass")
	return report
}

// A race's writers wait for each other on a database whose transactions meet: on a rolled-back env's
// one connection they would wait for good, so each race helper refuses that env at once and says
// which one it takes.
func TestARaceRefusesARolledBackEnv(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	nothing := testhelper.Writer{Name: "nothing", Run: func() error { return nil }}
	races := map[string]func(tb testing.TB){
		"QueueBehind": func(tb testing.TB) {
			tb.Helper()
			env.QueueBehind(tb, func() {}, nothing)
		},
		"QueueBehindWorkspaceLock": func(tb testing.TB) {
			tb.Helper()
			env.QueueBehindWorkspaceLock(tb, s, nothing)
		},
		"QueueBehindKey": func(tb testing.TB) {
			tb.Helper()
			env.QueueBehindKey(tb, 1, nothing)
		},
		"HoldOpen": func(tb testing.TB) {
			tb.Helper()
			env.HoldOpen(tb, s, func(*ent.Scoped, events.Publisher) error { return nil })
		},
	}
	for name, race := range races {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, failing(t, race), "testhelper.Committed()")
		})
	}
}

// A writer queued behind a hold waits for the hold: one that waits for another transaction held
// open, which letting the hold go never ends, fails the race at once instead of passing it.
func TestAWriterWaitingForAnotherTransactionFailsTheRace(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	held, other := postgres.WorkspaceKey("held", fixtures.AcmeID), postgres.WorkspaceKey("other", fixtures.AcmeID)
	elsewhere := testhelper.Writer{Name: "elsewhere", Run: func() error {
		// A transaction opened after the hold takes the other key and stays open until cleanup.
		owner, err := env.SQLDB.BeginTx(t.Context(), nil)
		if err != nil {
			return err
		}
		t.Cleanup(func() { _ = owner.Rollback() })
		if err := postgres.LockTx(t.Context(), owner, other); err != nil {
			return err
		}
		waiter, err := env.SQLDB.BeginTx(t.Context(), nil)
		if err != nil {
			return err
		}
		defer func() { _ = waiter.Rollback() }()
		return postgres.LockTx(t.Context(), waiter, other)
	}}

	report := failing(t, func(tb testing.TB) {
		tb.Helper()
		env.QueueBehindKey(tb, held, elsewhere)
	})

	assert.Contains(t, report, "the hold does not stand in")
}

// A writer that waits for the held key is queued behind it and finishes once it is let go.
func TestAWriterWaitingForTheHoldIsQueueBehindIt(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	held := postgres.WorkspaceKey("held", fixtures.AcmeID)
	behind := testhelper.Writer{Name: "behind", Run: func() error {
		tx, err := env.SQLDB.BeginTx(t.Context(), nil)
		if err != nil {
			return err
		}
		if err := postgres.LockTx(t.Context(), tx, held); err != nil {
			return err
		}
		return tx.Commit()
	}}

	errs := env.QueueBehindKey(t, held, behind)

	require.NoError(t, errs["behind"])
}
