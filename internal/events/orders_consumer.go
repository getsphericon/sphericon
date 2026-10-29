package events

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/ThreeDotsLabs/watermill/message"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/orders"
	"github.com/getsphericon/sphericon/internal/postgres"
)

// OrderRecorder applies an order Event to the Workspace behind s, inside a transaction. The
// orders subscriber delegates to it, like the webhooks subscriber does to its dispatcher.
type OrderRecorder interface {
	Record(ctx context.Context, s *ent.Scoped, in orders.Input) error
	// Resolve reads the Orders Completed that were held for want of a Reporting currency.
	Resolve(ctx context.Context, s *ent.Scoped, findClick orders.ClickFinder) error
}

// ordersConsumer reads the order Events (ADR 0064) into Orders. Like the other subscribers it
// is a raw-client entry: an envelope carries only a Workspace id, so the scope is built here,
// in the transaction of the write. A Backfilled Event is read too: a Backfill triggers
// Automations and Rules, not this consumer.
func ordersConsumer(client *ent.Client, recorder OrderRecorder) message.NoPublishHandlerFunc {
	return func(msg *message.Message) error {
		var env Envelope
		if err := json.Unmarshal(msg.Payload, &env); err != nil {
			return fmt.Errorf("unmarshal envelope: %w", err)
		}
		return RecordOrder(msg.Context(), client, recorder, env)
	}
}

// RecordOrder is the orders subscriber's work on one envelope; exported, like Persist and
// Suppress, so it can be driven without running the router. An order Event is recorded; the
// Audit entry of a change of the Reporting currency makes the Orders held for want of one be read.
func RecordOrder(ctx context.Context, client *ent.Client, recorder OrderRecorder, env Envelope) error {
	ev, err := Decode(env)
	if err != nil {
		return err
	}
	var work func(s *ent.Scoped) error
	switch e := ev.(type) {
	case *CollectedEvent:
		if !orders.IsOrderAction(e.Action) {
			return nil
		}
		p := e.Project()
		in := orders.Input{
			Action: p.Action, Properties: p.Properties, ContactID: p.ContactID, OccurredAt: env.OccurredAt,
			EventID: cmp.Or(env.DedupKey, env.ID), FindClick: LastClick,
		}
		work = func(s *ent.Scoped) error { return recorder.Record(ctx, s, in) }
	case *AuditEntry:
		if _, chosen := e.Diff[workspace.FieldReportingCurrency]; !chosen || e.Action != ActionWorkspaceUpdate {
			return nil
		}
		work = func(s *ent.Scoped) error { return recorder.Resolve(ctx, s, LastClick) }
	default:
		return nil
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin order transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// One transaction at a time reads the order Events of a Workspace: an Order Completed that
	// creates an Order and an Event that finds it missing and is held would otherwise cross, and
	// the held one would wait for ever. The Workspace's settings are read after the lock too, so
	// a Reporting currency chosen meanwhile is seen by whichever reads second.
	if err := postgres.LockTx(ctx, tx.Client(), orderLockKey(env.WorkspaceID)); err != nil {
		return fmt.Errorf("lock the orders of the Workspace: %w", err)
	}
	// The Workspace comes from the event envelope: a consumer has no membership or api token.
	if err := work(tx.Client().Scoped(env.WorkspaceID)); err != nil {
		return err
	}
	return tx.Commit()
}

// orderLockKey is the advisory lock of one Workspace's order Events.
func orderLockKey(workspaceID int64) int64 {
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "orders:%d", workspaceID)
	return int64(h.Sum32())
}

// LastClick is the latest human `email.clicked` Event of the Contact within [since, until] that
// carries a Broadcast or an Automation (ADR 0064); nil for none. A click flagged `machine`
// (ADR 0053) and a click of neither (a Transactional send) are passed over; equal times go to the
// click Event stored last.
func LastClick(ctx context.Context, s *ent.Scoped, contactID int64, since, until time.Time) (*orders.Click, error) {
	var found []*ent.Event
	err := s.Event().Query().
		Where(event.ContactID(contactID), event.ActionEQ(NameEmailClicked)).
		Modify(func(sel *sql.Selector) {
			HumanOnly(sel)
			props := sel.C(event.FieldProperties)
			// raw sql: a click carries the id of its Broadcast or Automation as a key of the JSON properties.
			sel.Where(sql.ExprP("(" + props + "->>'broadcastId' IS NOT NULL OR " + props + "->>'automationId' IS NOT NULL)"))
			moment := Moment(sel)
			// raw sql: the bounds are on the Moment expression, which ent's predicates cannot take as an operand.
			sel.Where(sql.P(func(b *sql.Builder) {
				b.WriteString(moment).WriteString(" >= ").Arg(since).
					WriteString(" AND ").WriteString(moment).WriteString(" <= ").Arg(until)
			}))
			// raw sql: latest first, ties to the Event stored last.
			sel.OrderExpr(sql.Expr(moment + " DESC, " + sel.C(event.FieldID) + " DESC"))
			sel.Limit(1)
		}).
		Scan(ctx, &found)
	if err != nil || len(found) == 0 {
		return nil, err
	}
	click := found[0]
	at := click.CreatedAt
	if click.OccurredAt != nil {
		at = *click.OccurredAt
	}
	owner := func(key string) int64 {
		switch v := click.Properties[key].(type) {
		case float64:
			return int64(v)
		case string:
			if id, err := strconv.ParseInt(v, 10, 64); err == nil {
				return id
			}
		}
		return 0
	}
	return &orders.Click{BroadcastID: owner("broadcastId"), AutomationID: owner("automationId"), At: at}, nil
}
