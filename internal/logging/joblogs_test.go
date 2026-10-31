package logging_test

import (
	"context"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/logging"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

type loggedArgs struct{}

func (loggedArgs) Kind() string { return "logged" }

// A line a job logs through the process's logger, with the job's context, is kept on the job,
// where the job queue UI shows it.
func TestAJobKeepsTheLinesItLogged(t *testing.T) {
	db := testhelper.RealDB(t)
	logger := logging.New(&config.Config{LogLevel: "info"})
	worker := river.WorkFunc(func(ctx context.Context, _ *river.Job[loggedArgs]) error {
		logger.InfoContext(ctx, "sent the welcome mail", "to", "ada@example.com")
		return nil
	})
	w := rivertest.NewWorker(t, riverdatabasesql.New(db), &river.Config{Middleware: []rivertype.Middleware{logging.JobLogs()}}, worker)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	res, err := w.Work(t.Context(), t, tx, loggedArgs{}, nil)

	require.NoError(t, err)
	assert.Contains(t, string(res.Job.Metadata), "sent the welcome mail")
}
