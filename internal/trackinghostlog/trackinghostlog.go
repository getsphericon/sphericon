// Package trackinghostlog writes the change log behind the service read of Tracking
// domain hosts (ADR 0038, ADR 0071). A Tracking domain is a verified host while its state is
// dns_verified or serving, whatever its role: a retired host is still served (Serving and Serves, the one
// definition the host gate shares). Each change of that (a host entering a state, or leaving
// both because its record stopped resolving or its row was deleted) is one row of
// tracking_host_changes, so a removal is never silence.
//
// Two ent hooks keep it so no write path can forget it, the same pair as
// internal/workspacefeed. Hook runs on every transaction client (the event bus's): the
// change and its log row commit together, in the order the positions were taken. Guard
// runs on the plain client (db.NewEntClient) and refuses a change to a host outside a
// transaction, where the log row would be a separate statement that a crash loses.
// The hook reads the Tracking domains a change touches by id, across Workspaces for a bulk
// write, which is why this package is on the closed list of raw-client packages in
// docs/architecture.md.
package trackinghostlog

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"entgo.io/ent"

	entclient "github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/trackingdomain"
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
// any row: Hook narrows it to the rows whose host actually changes. The host and the
// Workspace of a row are immutable, so only its state and its existence matter.
func changesHosts(m ent.Mutation) bool {
	mu, ok := m.(*entclient.TrackingDomainMutation)
	if !ok {
		return false
	}
	switch {
	case mu.Op().Is(ent.OpCreate):
		state, set := mu.State()
		return set && active(state)
	case mu.Op().Is(ent.OpUpdate | ent.OpUpdateOne):
		_, state := mu.State()
		return state
	}
	return mu.Op().Is(ent.OpDelete | ent.OpDeleteOne)
}

// Hook is the ent hook to install on a client bound to a transaction (Client.Use). It
// is a no-op for every mutation that cannot change what the log reports.
func Hook(next ent.Mutator) ent.Mutator {
	return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
		mu, ok := m.(*entclient.TrackingDomainMutation)
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
			host, _ := mu.Host()
			state, _ := mu.State()
			return v, record(ctx, c, change{workspaceID: workspaceID, host: host, state: trackinghostchange.State(state)})
		}

		ids, err := mu.IDs(ctx)
		if err != nil {
			return nil, err
		}
		before, err := c.TrackingDomain.Query().Where(trackingdomain.IDIn(ids...)).All(ctx)
		if err != nil {
			return nil, err
		}
		v, err := next.Mutate(ctx, m)
		if err != nil {
			return v, err
		}
		after := map[int64]*entclient.TrackingDomain{}
		if !mu.Op().Is(ent.OpDelete | ent.OpDeleteOne) {
			rows, err := c.TrackingDomain.Query().Where(trackingdomain.IDIn(ids...)).All(ctx)
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
	host        string
	state       trackinghostchange.State
}

// servingStates are the states in which a Tracking domain's host is verified.
var servingStates = []trackingdomain.State{
	trackingdomain.StateDNSVerified,
	trackingdomain.StateServing,
}

func active(s trackingdomain.State) bool {
	return slices.Contains(servingStates, s)
}

// Serving selects the Tracking domains whose host is served: a verified state, in any role.
// It is the query form of Serves; the host gate and the recheck job use it, so there is one
// definition.
func Serving() predicate.TrackingDomain {
	return trackingdomain.StateIn(servingStates...)
}

// Serves reports whether the row is a verified host: the row form of Serving.
func Serves(d *entclient.TrackingDomain) bool {
	return d != nil && active(d.State)
}

// diff is the log entries one row's change produces; now is nil when the row is gone.
func diff(old, now *entclient.TrackingDomain) []change {
	var out []change
	if Serves(old) && !Serves(now) {
		out = append(out, change{workspaceID: old.WorkspaceID, host: old.Host, state: trackinghostchange.StateRemoved})
	}
	if Serves(now) && (!Serves(old) || now.State != old.State) {
		out = append(out, change{workspaceID: now.WorkspaceID, host: now.Host, state: trackinghostchange.State(now.State)})
	}
	return out
}

// record appends the entries, taking the lock first, held to the end of the
// surrounding transaction.
func record(ctx context.Context, c *entclient.Client, changes ...change) error {
	if len(changes) == 0 {
		return nil
	}
	if err := postgres.LockTx(ctx, c, lockKey); err != nil {
		return fmt.Errorf("tracking host log lock: %w", err)
	}
	for _, ch := range changes {
		if err := c.TrackingHostChange.Create().
			SetWorkspaceID(ch.workspaceID).
			SetHost(ch.host).
			SetState(ch.state).
			Exec(ctx); err != nil {
			return fmt.Errorf("tracking host log: %w", err)
		}
	}
	return nil
}
