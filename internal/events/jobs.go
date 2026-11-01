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

// JobQueue is the queue inside the caller's transaction: the river client over database/sql
// satisfies it. A job inserted joins the transaction, so it exists iff that transaction
// commits; a delete (Erasure's purge, see QueuePurger) commits or rolls back with it too.
type JobQueue interface {
	InsertTx(ctx context.Context, tx *sql.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
	JobDeleteManyTx(ctx context.Context, tx *sql.Tx, params *river.JobDeleteManyParams) (*river.JobDeleteManyResult, error)
}

// Enqueuer puts a job on the queue inside the open transaction. Only WithinScopedEnqueueTx
// hands one out, so a state change and the job it needs commit or roll back together: no
// claim is left behind by a job that was never queued, and no job runs for a claim that
// was never written.
type Enqueuer interface {
	Enqueue(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error
}

// UseJobs gives the Bus the queue its transactional enqueues insert into and its purges
// delete from. Register it at composition time, before the bus is shared with concurrent
// callers.
func (b *Bus) UseJobs(q JobQueue) {
	b.jobs = q
}

// WithinScopedEnqueueTx is WithinScopedTx for a caller that also queues a job in the
// same transaction: a Broadcast's claim and its send job, an Enrollment's move and its next step.
func (b *Bus) WithinScopedEnqueueTx(ctx context.Context, s *ent.Scoped, fn func(ts *ent.Scoped, pub Publisher, q Enqueuer) error) error {
	return b.within(ctx, func(tx *ent.Client, pub *txPublisher) error {
		return fn(s.InTx(tx, auditPublisher{pub: pub}), pub, txQueue{jobs: pub.jobs, tx: pub.tx})
	})
}

type txQueue struct {
	jobs JobQueue
	tx   *sql.Tx
}

func (q txQueue) Enqueue(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error {
	if q.jobs == nil {
		return ErrNoJobQueue
	}
	_, err := q.jobs.InsertTx(ctx, q.tx, args, opts)
	return err
}
