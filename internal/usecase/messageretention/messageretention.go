// Package messageretention is the use case of message retention (ADR 0143): it deletes the
// Outbound messages, Broadcast recipients and tracked links older than the window and keeps the
// figures they made. A finished Broadcast is frozen whole (broadcastsunchecked), an Automation's
// old messages are counted into its figures as they go (automationsunchecked), and a Transactional
// message just goes; the deletes are Outbound send's (outboundmessagesunchecked).
//
// The work fans out by Workspace (ADR 0144): Due lists the Workspaces with something past the
// window, and Prune works one of them in batches, a transaction each, whose results persist at
// once. A run that stops, by its budget of batches or by a timeout, loses at most the batch in
// flight, and the next continues from what is left: frozen figures stay frozen, deleted rows stay
// deleted. The use case takes the raw client, since Due reads across every Workspace.
package messageretention

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastfigure"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations/automationsunchecked"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts/broadcastsunchecked"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages/outboundmessagesunchecked"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// Batch is the rows one transaction removes: few enough that a large backlog never holds a long
// lock.
const Batch = 2000

// Budget is the batches one run of Prune works before it stops and says more is left: enough to
// drain a day of a large Workspace's sends in a few runs, few enough for a run to fit its job's
// timeout.
const Budget = 100

// UseCase is message retention over the raw client and the event bus.
type UseCase struct {
	client    *ent.Client
	bus       *events.Bus
	retention time.Duration
	batch     int
	now       func() time.Time
}

// New returns the use case deleting messages older than retention (0 disables it), batch rows to a
// transaction (Batch; a test passes 1 to turn every loop), by the Instance clock now.
func New(client *ent.Client, bus *events.Bus, retention time.Duration, batch int, now func() time.Time) *UseCase {
	return &UseCase{client: client, bus: bus, retention: retention, batch: batch, now: now}
}

// Pruned is how many Outbound messages a run deleted, by kind, and whether its budget ran out
// before the Workspace's work did.
type Pruned struct {
	Broadcast, Automation, Transactional int
	More                                 bool
}

// Due lists the Workspaces with something past the window: a finished Broadcast not pruned yet, or
// an Automation or Transactional message that has left pending.
func (u *UseCase) Due(ctx context.Context) ([]int64, error) {
	if u.retention <= 0 {
		return nil, nil
	}
	cutoff := u.now().Add(-u.retention)
	var withBroadcasts []int64
	err := u.client.Broadcast.Query().
		Where(finishedBefore(cutoff)).
		Unique(true).
		Select(broadcast.FieldWorkspaceID).
		Scan(ctx, &withBroadcasts)
	if err != nil {
		return nil, err
	}
	var withMessages []int64
	err = u.client.OutboundMessage.Query().
		Where(
			outboundmessage.KindIn(outboundmessage.KindAutomation, outboundmessage.KindTransactional),
			outboundmessage.CreatedAtLT(cutoff),
			outboundmessage.StatusNEQ(outboundmessage.StatusPending),
		).
		Unique(true).
		Select(outboundmessage.FieldWorkspaceID).
		Scan(ctx, &withMessages)
	if err != nil {
		return nil, err
	}
	due := slices.Concat(withBroadcasts, withMessages)
	slices.Sort(due)
	return slices.Compact(due), nil
}

// finishedBefore matches the Broadcasts sent or failed (both final, ADR 0046) before cutoff and
// not pruned yet.
func finishedBefore(cutoff time.Time) predicate.Broadcast {
	return broadcast.And(
		broadcast.Or(
			broadcast.And(broadcast.StatusEQ(broadcast.StatusSent), broadcast.SentAtLT(cutoff)),
			broadcast.And(broadcast.StatusEQ(broadcast.StatusFailed), broadcast.FailedAtLT(cutoff)),
		),
		broadcast.Not(broadcast.HasFiguresWith(broadcastfigure.PrunedAtNotNil())),
	)
}

// Prune works the Workspace's messages past the window, at most budget batches, and returns how
// many went. It is idempotent: a run cut short leaves frozen figures that the next one keeps.
func (u *UseCase) Prune(ctx context.Context, workspaceID int64, budget int) (Pruned, error) {
	var p Pruned
	if u.retention <= 0 {
		return p, nil
	}
	r := run{UseCase: u, s: u.client.Scoped(workspaceID), now: u.now(), budget: budget}
	r.cutoff = r.now.Add(-u.retention)
	var err error
	if p.Broadcast, err = r.broadcasts(ctx); err != nil {
		return p, fmt.Errorf("prune finished broadcasts: %w", err)
	}
	if p.Automation, err = r.kind(ctx, outboundmessage.KindAutomation, automationsunchecked.AddPruned); err != nil {
		return p, fmt.Errorf("prune automation messages: %w", err)
	}
	if p.Transactional, err = r.kind(ctx, outboundmessage.KindTransactional, nil); err != nil {
		return p, fmt.Errorf("prune transactional messages: %w", err)
	}
	p.More = r.budget <= 0
	return p, nil
}

// run is one Prune of one Workspace: its scope, its window and the batches it has left.
type run struct {
	*UseCase

	s           *ent.Scoped
	now, cutoff time.Time
	budget      int
}

// spend takes one batch from the budget, false once it is spent.
func (r *run) spend() bool {
	if r.budget <= 0 {
		return false
	}
	r.budget--
	return true
}

// broadcasts freezes and deletes the Workspace's finished Broadcasts not pruned yet.
func (r *run) broadcasts(ctx context.Context) (int, error) {
	finished, err := r.s.Broadcast().QueryWithArchived().Where(finishedBefore(r.cutoff)).IDs(ctx)
	if err != nil {
		return 0, err
	}
	var total int
	for _, id := range finished {
		n, err := r.broadcast(ctx, id)
		total += n
		if err != nil {
			return total, fmt.Errorf("broadcast %d: %w", id, err)
		}
	}
	return total, nil
}

// broadcast freezes one finished Broadcast's figures, deletes its recipients each with its
// messages, then the messages left, and marks it pruned once nothing is.
func (r *run) broadcast(ctx context.Context, id int64) (int, error) {
	if err := broadcastsunchecked.Freeze(ctx, r.s, id); err != nil {
		return 0, err
	}
	var total int
	for {
		if !r.spend() {
			return total, nil
		}
		var ids []int64
		var deleted int
		err := r.bus.WithinAuditTx(ctx, r.s, func(ts *ent.Scoped) error {
			var err error
			if ids, err = broadcastsunchecked.RecipientsOf(ctx, ts, id, r.batch); err != nil || len(ids) == 0 {
				return err
			}
			if deleted, err = outboundmessagesunchecked.DeleteOfRecipients(ctx, ts, ids); err != nil {
				return err
			}
			return broadcastsunchecked.DeleteRecipients(ctx, ts, ids)
		})
		if err != nil {
			return total, err
		}
		total += deleted
		if len(ids) < r.batch {
			break
		}
	}
	for {
		if !r.spend() {
			return total, nil
		}
		n, err := outboundmessagesunchecked.DeleteOfBroadcast(ctx, r.s, id, r.batch)
		total += n
		if err != nil {
			return total, err
		}
		if n < r.batch {
			break
		}
	}
	return total, broadcastsunchecked.Pruned(ctx, r.s, id, r.now)
}

// kind deletes the Workspace's messages of the kind created before the cutoff that have left
// pending, a batch to a transaction; keep, when set, counts each batch in the transaction that
// deletes it.
func (r *run) kind(ctx context.Context, kind outboundmessage.Kind, keep func(context.Context, *ent.Scoped, []*ent.OutboundMessage) error) (int, error) {
	var total int
	for r.spend() {
		var taken []*ent.OutboundMessage
		err := r.bus.WithinAuditTx(ctx, r.s, func(ts *ent.Scoped) error {
			var err error
			if taken, err = outboundmessagesunchecked.TakeExpired(ctx, ts, kind, r.cutoff, r.batch); err != nil || len(taken) == 0 || keep == nil {
				return err
			}
			return keep(ctx, ts, taken)
		})
		if err != nil {
			return total, err
		}
		total += len(taken)
		if len(taken) < r.batch {
			break
		}
	}
	return total, nil
}
