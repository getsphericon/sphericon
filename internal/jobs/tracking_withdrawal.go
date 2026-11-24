package jobs

import (
	"context"
	stdsql "database/sql"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/usecase/trackingwithdrawal"
)

// TrackingWithdrawalArgs is a refusal's reach-back (ADR 0120): a Destination refused tracking, and
// what was collected of its earlier Outbound messages goes. The publish of the refusal's Event
// queues it in the refusal's transaction. It is keyed by its args, the Workspace, the Channel and
// the Destination: one waits or runs at a time per Destination, and a refusal recorded again
// meanwhile adds none, the one queued reaching every message the Destination has. Like every event
// consumer's job, it runs in the events queue. Its Destination is personal data: an Erasure of the
// Destination's Contact deletes the job (jobkind.TrackingWithdrawal) and does its work itself.
type TrackingWithdrawalArgs struct {
	jobkind.Workspace

	Channel     channels.Channel `json:"channel"`
	Destination string           `json:"destination"`
}

func (TrackingWithdrawalArgs) Kind() string { return jobkind.TrackingWithdrawal.Kind }

func (TrackingWithdrawalArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: events.QueueEvents, UniqueOpts: jobkind.QueuedOnce()}
}

// withdrawsTracking wants a refusal of tracking, on any Channel and from any provenance.
func withdrawsTracking(_ events.Envelope, ev events.DomainEvent) bool {
	_, ok := ev.(*events.EngagementTrackingDenied)
	return ok
}

// withdrawTracking is the reach-back the refusal calls for.
func withdrawTracking(env events.Envelope, ev events.DomainEvent) river.JobArgs {
	e, _ := ev.(*events.EngagementTrackingDenied) // withdrawsTracking established the type.
	return TrackingWithdrawalArgs{WorkspaceID: env.WorkspaceID, Channel: e.Channel, Destination: e.Destination}
}

// trackingWithdrawalTimeout bounds one run of the reach-back: a Destination with a long history is
// many batches, each persisted at once, so a run cut short resumes from what is left.
const trackingWithdrawalTimeout = 10 * time.Minute

// TrackingWithdrawalWorker runs the reach-back of a refusal in the Workspace the args name. It is a
// raw-client entry: the args carry only the Workspace's id, so the scope is built here.
type TrackingWithdrawalWorker struct {
	river.WorkerDefaults[TrackingWithdrawalArgs]

	ent        *ent.Client
	withdrawal *trackingwithdrawal.UseCase
}

// NewTrackingWithdrawalWorker is the reach-back's worker over the raw client and the use case.
func NewTrackingWithdrawalWorker(client *ent.Client, withdrawal *trackingwithdrawal.UseCase) *TrackingWithdrawalWorker {
	return &TrackingWithdrawalWorker{ent: client, withdrawal: withdrawal}
}

func (*TrackingWithdrawalWorker) Timeout(*river.Job[TrackingWithdrawalArgs]) time.Duration {
	return trackingWithdrawalTimeout
}

func (w *TrackingWithdrawalWorker) Work(ctx context.Context, job *river.Job[TrackingWithdrawalArgs]) error {
	return w.withdrawal.Withdraw(ctx, w.ent.Scoped(job.Args.WorkspaceID), trackingwithdrawal.Refusal{
		Channel: job.Args.Channel, Destination: job.Args.Destination,
	})
}

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
