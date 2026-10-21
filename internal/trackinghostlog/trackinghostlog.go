// Package trackinghostlog writes the change log behind the service read of Tracking
// domain hosts (ADR 0038). A Sending domain is a verified host while its tracking_state
// is dns_verified or serving and it is not archived (Serving, the one definition the host
// gate shares); each change of that (a host entering a state, or leaving both because its
// record stopped resolving, or its Sending domain was archived, deleted or renamed; an
// unarchive is an addition once the host is verified again) is one row of
// tracking_host_changes, so a removal is never silence.
//
// Two ent hooks keep it so no write path can forget it, the same pair as
// internal/workspacefeed. Hook runs on every transaction client (the event bus's): the
// change and its log row commit together, in the order the positions were taken. Guard
// runs on the plain client (db.NewEntClient) and refuses a change to a host outside a
// transaction, where the log row would be a separate statement that a crash loses.
// The hook reads Sending domains of any Workspace to find the hosts a change touches,
// which is why this package is on the closed list of raw-client packages in AGENTS.md.
package trackinghostlog

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"entgo.io/ent"

	entclient "github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/ent/trackinghostchange"
	"github.com/getsphericon/sphericon/internal/postgres"
)

// lockKey is the advisory lock that makes log ids commit in the order they are taken:
// it is held to the end of the transaction, so a reader that passed an id cannot miss
// an entry committed below it.
const lockKey = 7_204_002

// ErrNeedsTransaction is what Guard returns for a change to a host made on a client
// that is not bound to a transaction.
var ErrNeedsTransaction = errors.New("tracking host change outside a transaction")

// Guard is the ent hook for a client that is not bound to a transaction: it refuses
// every mutation that Hook would log. Such a change goes through the event bus's
// transaction (events.Bus.WithinTx).
func Guard(next ent.Mutator) ent.Mutator {
	return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
		if changesHosts(m) {
			return nil, ErrNeedsTransaction
		}
		return next.Mutate(ctx, m)
	})
}

// changesHosts reports whether m can change what the log reports, before looking at
// any row: Hook narrows it to the rows whose host actually changes.
func changesHosts(m ent.Mutation) bool {
	mu, ok := m.(*entclient.SendingDomainMutation)
	if !ok {
		return false
	}
	switch {
	case mu.Op().Is(ent.OpCreate):
		state, set := mu.TrackingState()
		return set && active(state)
	case mu.Op().Is(ent.OpUpdate | ent.OpUpdateOne):
		_, state := mu.TrackingState()
		_, domain := mu.Domain()
		_, archived := mu.DiscardedAt()
		return state || domain || archived || mu.DiscardedAtCleared()
	}
	return mu.Op().Is(ent.OpDelete | ent.OpDeleteOne)
}

// Hook is the ent hook to install on a client bound to a transaction (Client.Use). It
// is a no-op for every mutation that cannot change what the log reports.
func Hook(next ent.Mutator) ent.Mutator {
	return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
		mu, ok := m.(*entclient.SendingDomainMutation)
		if !ok || !changesHosts(m) {
			return next.Mutate(ctx, m)
		}
		c := mu.Client()
		if mu.Op().Is(ent.OpCreate) {
			v, err := next.Mutate(ctx, m)
			if err != nil {
				return v, err
			}
			workspaceID, _ := mu.WorkspaceID()
			domain, _ := mu.Domain()
			state, _ := mu.TrackingState()
			return v, record(ctx, c, change{workspaceID: workspaceID, domain: domain, state: trackinghostchange.State(state)})
		}

		ids, err := mu.IDs(ctx)
		if err != nil {
			return nil, err
		}
		before, err := c.SendingDomain.Query().Where(sendingdomain.IDIn(ids...)).All(ctx)
		if err != nil {
			return nil, err
		}
		v, err := next.Mutate(ctx, m)
		if err != nil {
			return v, err
		}
		after := map[int64]*entclient.SendingDomain{}
		if !mu.Op().Is(ent.OpDelete | ent.OpDeleteOne) {
			rows, err := c.SendingDomain.Query().Where(sendingdomain.IDIn(ids...)).All(ctx)
			if err != nil {
				return nil, err
			}
			for _, r := range rows {
				after[r.ID] = r
			}
		}
		var changes []change
		for _, old := range before {
			changes = append(changes, diff(old, after[old.ID])...)
		}
		return v, record(ctx, c, changes...)
	})
}

type change struct {
	workspaceID int64
	domain      string
	state       trackinghostchange.State
}

// servingStates are the tracking states in which a Sending domain's host is verified.
var servingStates = []sendingdomain.TrackingState{
	sendingdomain.TrackingStateDNSVerified,
	sendingdomain.TrackingStateServing,
}

func active(s sendingdomain.TrackingState) bool {
	return slices.Contains(servingStates, s)
}

// Serving selects the Sending domains that serve their Tracking host: a verified tracking
// state and not archived (ADR 0049). It is the query form of serves; the host gate and the
// recheck job use it, so there is one definition.
func Serving() predicate.SendingDomain {
	return sendingdomain.And(sendingdomain.DiscardedAtIsNil(), sendingdomain.TrackingStateIn(servingStates...))
}

// serves reports whether the row is a verified host: the row form of Serving.
func serves(d *entclient.SendingDomain) bool {
	return d != nil && d.DiscardedAt == nil && active(d.TrackingState)
}

// diff is the log entries one row's change produces; now is nil when the row is gone.
func diff(old, now *entclient.SendingDomain) []change {
	moved := now == nil || now.Domain != old.Domain || now.WorkspaceID != old.WorkspaceID
	var out []change
	if serves(old) && (moved || !serves(now)) {
		out = append(out, change{workspaceID: old.WorkspaceID, domain: old.Domain, state: trackinghostchange.StateRemoved})
	}
	if serves(now) && (!serves(old) || moved || now.TrackingState != old.TrackingState) {
		out = append(out, change{workspaceID: now.WorkspaceID, domain: now.Domain, state: trackinghostchange.State(now.TrackingState)})
	}
	return out
}

// record appends the entries, taking the lock first, held to the end of the
// surrounding transaction. A host created pending is not a change.
func record(ctx context.Context, c *entclient.Client, changes ...change) error {
	var logged []change
	for _, ch := range changes {
		if ch.state == trackinghostchange.StateDNSVerified || ch.state == trackinghostchange.StateServing || ch.state == trackinghostchange.StateRemoved {
			logged = append(logged, ch)
		}
	}
	if len(logged) == 0 {
		return nil
	}
	if err := postgres.LockTx(ctx, c, lockKey); err != nil {
		return fmt.Errorf("tracking host log lock: %w", err)
	}
	for _, ch := range logged {
		if err := c.TrackingHostChange.Create().
			SetWorkspaceID(ch.workspaceID).
			SetDomain(ch.domain).
			SetState(ch.state).
			Exec(ctx); err != nil {
			return fmt.Errorf("tracking host log: %w", err)
		}
	}
	return nil
}
