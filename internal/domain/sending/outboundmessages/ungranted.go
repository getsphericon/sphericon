package outboundmessages

import (
	"context"
	"slices"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// The withdrawal by the default (ADR 0120): a Workspace's Engagement tracking default turned to
// dont_track withdraws the tracking of every message already sent to a Destination without a
// granted, and deletes nothing: their Events, stamps and credits stay.

// UngrantedBatch is how many messages one transaction of the withdrawal by the default takes;
// UngrantedBudget is how many batches one run of its job works, so a run fits the job's timeout
// and the next run continues after the last message it reached.
const (
	UngrantedBatch  = 1000
	UngrantedBudget = 50
)

// WithdrawUngrantedArgs is the job of the withdrawal by the default: every message of the
// Workspace created up to Before, the moment the default was turned to dont_track, to a
// Destination without a granted. A run continues after the message id After. The turn queues it
// in its transaction; each turn queues its own, and running one over messages another withdrew
// withdraws nothing more.
type WithdrawUngrantedArgs struct {
	jobkind.Workspace

	Before time.Time `json:"before"`
	After  int64     `json:"after"`
}

func (WithdrawUngrantedArgs) Kind() string { return "tracking_withdrawal_ungranted" }

// UngrantedWithdrawal is a run of the withdrawal by the default: the messages created up to Before,
// after the message id After, in up to Batches transactions of Limit messages.
type UngrantedWithdrawal struct {
	Before  time.Time
	After   int64
	Limit   int
	Batches int
}

// WithdrawalProgress is how far a withdrawal got: the last message id it reached, and whether
// messages may be left after it.
type WithdrawalProgress struct {
	Last int64
	More bool
}

// WithdrawUngranted withdraws the tracking of the messages of the Workspace of s created up to
// in.Before whose Destination has no granted Engagement tracking consent on their Channel (ADR
// 0120), in up to in.Batches batches, each in a transaction of its own that persists at once
// (WithdrawUngrantedIn). It deletes nothing. It is idempotent: a message withdrawn stays so, and a
// run over it again withdraws nothing more.
func (m *Module) WithdrawUngranted(ctx context.Context, s *ent.Scoped, in UngrantedWithdrawal) (WithdrawalProgress, error) {
	out := WithdrawalProgress{Last: in.After, More: true}
	for range in.Batches {
		err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
			var err error
			out, err = m.WithdrawUngrantedIn(ctx, ts, UngrantedWithdrawal{Before: in.Before, After: out.Last, Limit: in.Limit})
			return err
		})
		if err != nil || !out.More {
			return out, err
		}
	}
	return out, nil
}

// WithdrawUngrantedIn is one batch of WithdrawUngranted, in the transaction ts is bound to
// (WithdrawTracking): up to in.Limit messages after the message id in.After, created up to
// in.Before, whose tracking stands and whose Destination has no granted on their Channel. The
// consent is read as it stands now: a Destination granted before the withdrawal reaches its
// messages keeps them tracked, and one granted after gets nothing back.
func (m *Module) WithdrawUngrantedIn(ctx context.Context, ts *ent.Scoped, in UngrantedWithdrawal) (WithdrawalProgress, error) {
	taken, err := m.WithdrawTracking(ctx, ts, WithdrawalBatch{
		Messages: outboundmessage.And(Tracked(), outboundmessage.CreatedAtLTE(in.Before), m.ungranted()),
		After:    in.After,
		Limit:    in.Limit,
	})
	if err != nil || len(taken) == 0 {
		return WithdrawalProgress{Last: in.After}, err
	}
	return WithdrawalProgress{Last: taken[len(taken)-1].ID, More: len(taken) == in.Limit}, nil
}

// ungranted selects the messages whose Destination has no granted on their Channel: the effective
// consent under a dont_track default (engagementtracking.UntrackedWhere), whatever the default is
// now. A Destination that refused has no granted either.
func (m *Module) ungranted() predicate.OutboundMessage {
	chs := lo.Keys(m.channels)
	slices.Sort(chs)
	return outboundmessage.Or(lo.Map(chs, func(ch channels.Channel, _ int) predicate.OutboundMessage {
		return outboundmessage.And(outboundmessage.ChannelEQ(ch),
			predicate.OutboundMessage(engagementtracking.UntrackedWhere(ch, engagementtracking.DontTrack, outboundmessage.FieldDestination)))
	})...)
}
