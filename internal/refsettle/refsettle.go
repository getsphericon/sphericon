// Package refsettle settles the rows whose reference points at a row that is gone, and then validates
// the foreign keys a migration added NOT VALID (ADR 0033: a migration only adds, the heavy data work
// runs afterwards). The references date from before they were foreign keys (an Erasure, an erased
// Template): a row either goes with the row it named (it died with it) or only the dangling id goes (it
// outlives it), the way the key's `ON DELETE` would have. A new write cannot dangle, since the keys are
// enforced from the migration on, so once a key is validated every run finds nothing.
//
// The settling reads and writes every Workspace's rows, so the module holds the raw ent client. The
// jobs only call it.
package refsettle

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
)

// Settler settles dangling references on the raw ent client.
type Settler struct{ client *ent.Client }

// New builds a Settler over the raw client.
func New(client *ent.Client) *Settler { return &Settler{client: client} }

// ref is one reference to a row: its table, the key the migration added, how to find the rows whose
// reference dangles (at most limit of them) and how to settle those rows.
type ref struct {
	table      string
	constraint string
	find       func(ctx context.Context, c *ent.Client, limit int) ([]int64, error)
	settle     func(ctx context.Context, c *ent.Client, ids []int64) (int, error)
}

// ClearContactRefs settles, table by table, the rows that name a Contact which is gone (at most
// batch rows per step, so no long lock; a run interrupted halfway finishes on the next), and
// validates each key that is still NOT VALID once its table is clean. It returns how many rows it
// settled.
func (s *Settler) ClearContactRefs(ctx context.Context, batch int) (int, error) {
	return s.run(ctx, batch, contactRefs)
}

// ClearOutboundMessageRefs clears, reference by reference, the ids of Outbound messages that point
// at nothing (at most batch rows per step, so no long lock; a run interrupted halfway finishes on
// the next), and validates each key that is still NOT VALID once its column is clean. It returns
// how many ids it cleared. A message keeps everything else: only the dangling id goes.
func (s *Settler) ClearOutboundMessageRefs(ctx context.Context, batch int) (int, error) {
	return s.run(ctx, batch, outboundMessageRefs)
}

func (s *Settler) run(ctx context.Context, batch int, refs []ref) (int, error) {
	total := 0
	for _, r := range refs {
		for {
			ids, err := r.find(ctx, s.client, batch)
			if err != nil {
				return total, fmt.Errorf("find %s: %w", r.constraint, err)
			}
			n, err := r.settle(ctx, s.client, ids)
			if err != nil {
				return total, fmt.Errorf("settle %s: %w", r.constraint, err)
			}
			total += n
			if len(ids) < batch {
				break
			}
		}
		if err := s.validate(ctx, r.table, r.constraint); err != nil {
			return total, err
		}
	}
	return total, nil
}

// validate validates one key of table that a migration added NOT VALID. The table and the
// constraint are constants of this package, never input.
func (s *Settler) validate(ctx context.Context, table, constraint string) error {
	// raw sql: reads pg_constraint to see whether the migration's key is still NOT VALID; the
	// catalog is not an ent entity.
	rows, err := s.client.QueryContext(ctx,
		`SELECT NOT convalidated FROM pg_constraint WHERE conname = $1 AND conrelid = $2::regclass`, constraint, table)
	if err != nil {
		return fmt.Errorf("look up %s: %w", constraint, err)
	}
	defer func() { _ = rows.Close() }()
	pending := false
	if rows.Next() {
		if err := rows.Scan(&pending); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !pending {
		return nil
	}
	// raw sql: the DDL that completes the migration's NOT VALID key once the data is clean
	// (ADR 0033); ent runs no DDL outside a migration. The names are constants of this package.
	if _, err := s.client.ExecContext(ctx, `ALTER TABLE `+table+` VALIDATE CONSTRAINT `+constraint); err != nil {
		return fmt.Errorf("validate %s: %w", constraint, err)
	}
	return nil
}

// contactRefs are the references to a Contact. A row that dies with its Contact is deleted, the rest
// lose only the id.
var contactRefs = []ref{
	{
		table: "automation_runs", constraint: "automation_runs_contacts_contact",
		find: func(ctx context.Context, c *ent.Client, limit int) ([]int64, error) {
			return c.AutomationRun.Query().Where(automationrun.Not(automationrun.HasContactWith())).Limit(limit).IDs(ctx)
		},
		settle: func(ctx context.Context, c *ent.Client, ids []int64) (int, error) {
			return c.AutomationRun.Delete().Where(automationrun.IDIn(ids...)).Exec(ctx)
		},
	},
	{
		table: "confirmations", constraint: "confirmations_contacts_contact",
		find: func(ctx context.Context, c *ent.Client, limit int) ([]int64, error) {
			return c.Confirmation.Query().
				Where(confirmation.ContactIDNotNil(), confirmation.Not(confirmation.HasContactWith())).Limit(limit).IDs(ctx)
		},
		settle: func(ctx context.Context, c *ent.Client, ids []int64) (int, error) {
			return c.Confirmation.Delete().Where(confirmation.IDIn(ids...)).Exec(ctx)
		},
	},
	{
		table: "unsubscribes", constraint: "unsubscribes_contacts_contact",
		find: func(ctx context.Context, c *ent.Client, limit int) ([]int64, error) {
			return c.Unsubscribe.Query().
				Where(unsubscribe.ContactIDNotNil(), unsubscribe.Not(unsubscribe.HasContactWith())).Limit(limit).IDs(ctx)
		},
		settle: func(ctx context.Context, c *ent.Client, ids []int64) (int, error) {
			return c.Unsubscribe.Update().Where(unsubscribe.IDIn(ids...)).ClearContactID().Save(ctx)
		},
	},
	{
		table: "suppressions", constraint: "suppressions_contacts_contact",
		find: func(ctx context.Context, c *ent.Client, limit int) ([]int64, error) {
			return c.Suppression.Query().
				Where(suppression.ContactIDNotNil(), suppression.Not(suppression.HasContactWith())).Limit(limit).IDs(ctx)
		},
		settle: func(ctx context.Context, c *ent.Client, ids []int64) (int, error) {
			return c.Suppression.Update().Where(suppression.IDIn(ids...)).ClearContactID().Save(ctx)
		},
	},
	{
		table: "broadcast_recipients", constraint: "broadcast_recipients_contacts_contact",
		find: func(ctx context.Context, c *ent.Client, limit int) ([]int64, error) {
			return c.BroadcastRecipient.Query().
				Where(broadcastrecipient.ContactIDNotNil(), broadcastrecipient.Not(broadcastrecipient.HasContactWith())).
				Limit(limit).IDs(ctx)
		},
		settle: func(ctx context.Context, c *ent.Client, ids []int64) (int, error) {
			return c.BroadcastRecipient.Update().Where(broadcastrecipient.IDIn(ids...)).ClearContactID().Save(ctx)
		},
	},
	{
		table: "outbound_messages", constraint: "outbound_messages_contacts_contact",
		find: func(ctx context.Context, c *ent.Client, limit int) ([]int64, error) {
			return c.OutboundMessage.Query().
				Where(outboundmessage.ContactIDNotNil(), outboundmessage.Not(outboundmessage.HasContactWith())).Limit(limit).IDs(ctx)
		},
		settle: func(ctx context.Context, c *ent.Client, ids []int64) (int, error) {
			return c.OutboundMessage.Update().Where(outboundmessage.IDIn(ids...)).ClearContactID().Save(ctx)
		},
	},
}

// outboundMessageRefs are the references of an Outbound message to the row that produced it. A
// message is a fact that outlives them all, so each loses only its id.
var outboundMessageRefs = []ref{
	outboundRef("outbound_messages_broadcasts_broadcast",
		outboundmessage.BroadcastIDNotNil(), outboundmessage.HasBroadcastWith(),
		(*ent.OutboundMessageUpdate).ClearBroadcastID),
	outboundRef("outbound_messages_broadcast_recipients_broadcast_recipient",
		outboundmessage.BroadcastRecipientIDNotNil(), outboundmessage.HasBroadcastRecipientWith(),
		(*ent.OutboundMessageUpdate).ClearBroadcastRecipientID),
	outboundRef("outbound_messages_automations_automation",
		outboundmessage.AutomationIDNotNil(), outboundmessage.HasAutomationWith(),
		(*ent.OutboundMessageUpdate).ClearAutomationID),
	outboundRef("outbound_messages_automation_runs_automation_run",
		outboundmessage.AutomationRunIDNotNil(), outboundmessage.HasAutomationRunWith(),
		(*ent.OutboundMessageUpdate).ClearAutomationRunID),
	outboundRef("outbound_messages_integrations_integration",
		outboundmessage.IntegrationIDNotNil(), outboundmessage.HasIntegrationWith(),
		(*ent.OutboundMessageUpdate).ClearIntegrationID),
	outboundRef("outbound_messages_email_templates_template",
		outboundmessage.TemplateIDNotNil(), outboundmessage.HasTemplateWith(),
		(*ent.OutboundMessageUpdate).ClearTemplateID),
}

// outboundRef is the reference of an Outbound message that is set (`set`), names a row that exists
// (`exists`) and is cleared by `unset`.
func outboundRef(constraint string, set, exists predicate.OutboundMessage, unset func(*ent.OutboundMessageUpdate) *ent.OutboundMessageUpdate) ref {
	return ref{
		table: "outbound_messages", constraint: constraint,
		find: func(ctx context.Context, c *ent.Client, limit int) ([]int64, error) {
			return c.OutboundMessage.Query().Where(set, outboundmessage.Not(exists)).Limit(limit).IDs(ctx)
		},
		settle: func(ctx context.Context, c *ent.Client, ids []int64) (int, error) {
			return unset(c.OutboundMessage.Update().Where(outboundmessage.IDIn(ids...))).Save(ctx)
		},
	}
}
