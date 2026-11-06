package testhelper

import (
	"context"
	"database/sql"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertest"

	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
)

// Work runs a copy of a job through rivertest on a connection of its own: rivertest inserts the
// copy, marks it running and completes it inside a transaction it is given, rolled back after the
// job. PerformEnqueuedJobs (perform.go) works the queued jobs themselves.

// Work runs one job of args through worker the way river would (args are encoded and decoded,
// the worker's timeout and the application's plugins and middleware apply, jobs.BaseConfig) and
// returns the error the worker returned.
func Work[T river.JobArgs](ctx context.Context, tb testing.TB, worker river.Worker[T], args T) error {
	tb.Helper()
	initBaseline()
	if errLoad != nil {
		return errLoad
	}
	pool, err := sql.Open("pgx", baseCfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer cleanup.Close(pool, "the pool of a test job")
	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	w := rivertest.NewWorker(tb, riverdatabasesql.New(pool), jobs.BaseConfig(), worker)
	_, err = w.Work(ctx, tb, tx, args, nil)
	return err
}
