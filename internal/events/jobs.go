package events

import (
	"context"
	"database/sql"
	"errors"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/getsphericon/sphericon/ent"
)

// ErrNoJobQueue is returned by a transactional enqueue on a Bus that was given no job queue.
var ErrNoJobQueue = errors.New("events: the bus has no job queue")

// JobInserter is the queue's transactional insert: the river client over database/sql
// satisfies it. The job row joins the caller's transaction, so it exists iff that
// transaction commits.
type JobInserter interface {
	InsertTx(ctx context.Context, tx *sql.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// Enqueuer puts a job on the queue inside the open transaction. Only WithinScopedEnqueueTx
// hands one out, so a state change and the job it needs commit or roll back together: no
// claim is left behind by a job that was never queued, and no job runs for a claim that
// was never written.
type Enqueuer interface {
	Enqueue(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error
}

// UseJobs gives the Bus the queue its transactional enqueues insert into. Register it at
// composition time, before the bus is shared with concurrent callers.
func (b *Bus) UseJobs(q JobInserter) {
	b.jobs = q
}

// WithinScopedEnqueueTx is WithinScopedTx for a caller that also queues a job in the
// same transaction: a Broadcast's claim and its send job.
func (b *Bus) WithinScopedEnqueueTx(ctx context.Context, s *ent.Scoped, fn func(ts *ent.Scoped, q Enqueuer) error) error {
	return b.within(ctx, func(tx *ent.Client, pub *txPublisher) error {
		return fn(s.InTx(tx, auditPublisher{pub: pub}), txQueue{jobs: b.jobs, tx: pub.tx})
	})
}

type txQueue struct {
	jobs JobInserter
	tx   *sql.Tx
}

func (q txQueue) Enqueue(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error {
	if q.jobs == nil {
		return ErrNoJobQueue
	}
	_, err := q.jobs.InsertTx(ctx, q.tx, args, opts)
	return err
}
