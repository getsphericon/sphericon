package jobs

import (
	"context"
	stdsql "database/sql"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
)

// WithdrawUngrantedWorker withdraws the tracking of a Workspace's messages to the Destinations
// without a granted once its Engagement tracking default is turned to dont_track (ADR 0120), in
// batches that persist at once. A run works at most a budget of batches, so it fits its timeout,
// and queues the next run, after the last message it reached, while more may be left.
type WithdrawUngrantedWorker struct {
	river.WorkerDefaults[outboundmessages.WithdrawUngrantedArgs]

	ent *ent.Client
	out *outboundmessages.Module
}

// NewWithdrawUngrantedWorker builds the worker over the Outbound message module.
func NewWithdrawUngrantedWorker(client *ent.Client, out *outboundmessages.Module) *WithdrawUngrantedWorker {
	return &WithdrawUngrantedWorker{ent: client, out: out}
}

func (w *WithdrawUngrantedWorker) Work(ctx context.Context, job *river.Job[outboundmessages.WithdrawUngrantedArgs]) error {
	got, err := w.out.WithdrawUngranted(ctx, w.ent.Scoped(job.Args.WorkspaceID), outboundmessages.UngrantedWithdrawal{
		Before: job.Args.Before, After: job.Args.After,
		Limit: outboundmessages.UngrantedBatch, Batches: outboundmessages.UngrantedBudget,
	})
	if err != nil || !got.More {
		return err
	}
	next := job.Args
	next.After = got.Last
	_, err = river.ClientFromContext[*stdsql.Tx](ctx).Insert(ctx, next, nil)
	return err
}
