package testhelper

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertest"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/domainverify"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/mailjobs"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/secrets"
)

// The jobs a request queues are worked at once, each by its real river worker through
// rivertest, so a test asserts what the job did (the mail sent, the row written) rather than
// that a job was queued. rivertest inserts the job, marks it running and completes it inside
// a transaction it is given; that transaction is on a connection of its own and rolled back
// after the job, because the test's database is one connection the worker's own queries need
// while the job runs.

// Work runs one job of args through worker the way river would (args are encoded and decoded,
// the worker's timeout and middleware apply) and returns the error the worker returned.
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
	w := rivertest.NewWorker(tb, riverdatabasesql.New(pool), &river.Config{}, worker)
	_, err = w.Work(ctx, tb, tx, args, nil)
	return err
}

// testJobs is the enqueue side of the job queue in a test (app.Enqueuer): each job is worked
// at once by its real worker.
type testJobs struct {
	tb      testing.TB
	ent     *ent.Client
	sender  messaging.EmailSender
	domains *domainverify.Module
	cipher  *secrets.Cipher
	catalog *messaging.Catalog
	appURL  string
}

// Enqueue works the job of args now.
func (j *testJobs) Enqueue(ctx context.Context, args river.JobArgs) error {
	switch a := args.(type) {
	case mailjobs.WelcomeArgs:
		return Work(ctx, j.tb, jobs.NewWelcomeWorker(j.sender), a)
	case mailjobs.AuthMailArgs:
		return Work(ctx, j.tb, jobs.NewAuthMailWorker(j.sender, j.appURL), a)
	case mailjobs.MemberInviteArgs:
		return Work(ctx, j.tb, jobs.NewMemberInviteWorker(j.sender), a)
	case mailjobs.SecondFactorRequiredArgs:
		return Work(ctx, j.tb, jobs.NewNotifySecondFactorRequiredWorker(j.ent, j.sender, j.appURL), a)
	case jobs.RefreshIntegrationQuotaArgs:
		return Work(ctx, j.tb, jobs.NewRefreshIntegrationQuotaWorker(j.ent, j.cipher, j.catalog), a)
	case jobs.VerifySendingDomainArgs:
		return Work(ctx, j.tb, jobs.NewVerifySendingDomainWorker(j.domains), a)
	default:
		return fmt.Errorf("testhelper: no worker for the job kind %q", args.Kind())
	}
}

// EnqueueIntegrationQuotaRefresh reads the Integration's provider send quota now.
func (j *testJobs) EnqueueIntegrationQuotaRefresh(ctx context.Context, integrationID int64) error {
	return j.Enqueue(ctx, jobs.RefreshIntegrationQuotaArgs{IntegrationID: integrationID})
}

// EnqueueSendingDomainVerify re-checks the Sending domain's DKIM now.
func (j *testJobs) EnqueueSendingDomainVerify(ctx context.Context, sendingDomainID int64) error {
	return j.Enqueue(ctx, jobs.VerifySendingDomainArgs{SendingDomainID: sendingDomainID})
}

// VerifySendingDomain runs both checks of the Sending domain now, as the river client does.
func (j *testJobs) VerifySendingDomain(ctx context.Context, sendingDomainID int64) error {
	return j.domains.Verify(ctx, sendingDomainID)
}
