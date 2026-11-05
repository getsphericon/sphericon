package jobs

import (
	"context"
	"database/sql"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/domainverify"
)

// Queue is the side of the job queue the modules and the /site handlers queue work through: an
// insert-only river client over the main pool and the checks a person waits for. It runs no
// worker, so what queues work does not depend on what the workers drive (Outbound send), and a
// Client that works the jobs, in this process or another, picks them up.
type Queue struct {
	river *river.Client[*sql.Tx]
	// domains runs the checks a person waits for (the Verify button of a Sending domain).
	domains *domainverify.Module
}

// NewQueue is the Queue over db; domains verifies a Sending domain on demand.
func NewQueue(db *sql.DB, domains *domainverify.Module) (*Queue, error) {
	inserter, err := NewInserter(db)
	if err != nil {
		return nil, err
	}
	return &Queue{river: inserter, domains: domains}, nil
}

// Enqueue queues one job of args. It is the one enqueue method the modules that queue
// platform mail use (mailjobs.Enqueuer).
func (q *Queue) Enqueue(ctx context.Context, args river.JobArgs) error {
	_, err := q.river.Insert(ctx, args, nil)
	return err
}

// EnqueueIntegrationQuotaRefresh schedules a read of one Integration's provider send
// quota (on save; the hourly tick covers the rest). Fire-and-forget: the result, or
// the warning that it could not be read, lands on the row.
func (q *Queue) EnqueueIntegrationQuotaRefresh(ctx context.Context, integrationID int64) error {
	_, err := q.river.Insert(ctx, RefreshIntegrationQuotaArgs{IntegrationID: integrationID}, nil)
	return err
}

// VerifySendingDomain runs both checks of one Sending domain now (the "Verify" button) and
// returns once they are done: the result is on the row.
func (q *Queue) VerifySendingDomain(ctx context.Context, sendingDomainID int64) error {
	return q.domains.Verify(ctx, sendingDomainID)
}
