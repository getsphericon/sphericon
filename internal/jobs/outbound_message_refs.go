package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
)

const (
	clearOutboundRefsInterval = time.Hour
	clearOutboundRefsBatch    = 1000
)

// ClearOutboundMessageRefsArgs is the periodic tick that clears the ids on Outbound messages that
// point at a row which is gone (an Automation run or a Template erased before the references were
// foreign keys), then validates the keys the migration added NOT VALID (ADR 0033: the migration
// only adds, this job is the heavy data work). A new write cannot dangle, since the keys are
// enforced from the migration on, so once a key has been validated every tick finds nothing.
type ClearOutboundMessageRefsArgs struct{}

func (ClearOutboundMessageRefsArgs) Kind() string { return "clear_outbound_message_refs" }

// ClearOutboundMessageRefsWorker runs ClearOutboundMessageRefs. It reads every Workspace's rows, so
// it holds the raw ent client.
type ClearOutboundMessageRefsWorker struct {
	river.WorkerDefaults[ClearOutboundMessageRefsArgs]
	ent *ent.Client
}

func (w *ClearOutboundMessageRefsWorker) Work(ctx context.Context, _ *river.Job[ClearOutboundMessageRefsArgs]) error {
	cleared, err := ClearOutboundMessageRefs(ctx, w.ent, clearOutboundRefsBatch)
	if cleared > 0 {
		slog.InfoContext(ctx, "dangling Outbound message references cleared", "ids", cleared)
	}
	return err
}

// outboundRef is one reference of an Outbound message: the key the migration added and the
// statement that clears the ids with no row behind them. The statements are literal, one per
// column, so nothing is spliced into SQL.
type outboundRef struct {
	constraint string
	// raw sql: a batched UPDATE ... WHERE NOT EXISTS over another table; ent updates do not
	// anti-join.
	clear string
}

var outboundRefs = []outboundRef{
	{"outbound_messages_broadcasts_broadcast", `
UPDATE outbound_messages SET broadcast_id = NULL WHERE id IN (
  SELECT m.id FROM outbound_messages m
  WHERE m.broadcast_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM broadcasts t WHERE t.id = m.broadcast_id)
  LIMIT $1)`},
	{"outbound_messages_broadcast_recipients_broadcast_recipient", `
UPDATE outbound_messages SET broadcast_recipient_id = NULL WHERE id IN (
  SELECT m.id FROM outbound_messages m
  WHERE m.broadcast_recipient_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM broadcast_recipients t WHERE t.id = m.broadcast_recipient_id)
  LIMIT $1)`},
	{"outbound_messages_automations_automation", `
UPDATE outbound_messages SET automation_id = NULL WHERE id IN (
  SELECT m.id FROM outbound_messages m
  WHERE m.automation_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM automations t WHERE t.id = m.automation_id)
  LIMIT $1)`},
	{"outbound_messages_automation_runs_automation_run", `
UPDATE outbound_messages SET automation_run_id = NULL WHERE id IN (
  SELECT m.id FROM outbound_messages m
  WHERE m.automation_run_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM automation_runs t WHERE t.id = m.automation_run_id)
  LIMIT $1)`},
	{"outbound_messages_integrations_integration", `
UPDATE outbound_messages SET integration_id = NULL WHERE id IN (
  SELECT m.id FROM outbound_messages m
  WHERE m.integration_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM integrations t WHERE t.id = m.integration_id)
  LIMIT $1)`},
	{"outbound_messages_email_templates_template", `
UPDATE outbound_messages SET template_id = NULL WHERE id IN (
  SELECT m.id FROM outbound_messages m
  WHERE m.template_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM email_templates t WHERE t.id = m.template_id)
  LIMIT $1)`},
}

// ClearOutboundMessageRefs clears, reference by reference, the ids that point at nothing (at most
// batch rows per statement, so no long lock; a run interrupted halfway finishes on the next), and
// validates each key that is still NOT VALID once its column is clean. It returns how many ids it
// cleared. A message keeps everything else: only the dangling id goes.
func ClearOutboundMessageRefs(ctx context.Context, client *ent.Client, batch int) (int, error) {
	total := 0
	for _, ref := range outboundRefs {
		for {
			// raw sql: the batched anti-join UPDATE of outboundRefs, one literal statement per column.
			n, err := affected(client.ExecContext(ctx, ref.clear, batch))
			if err != nil {
				return total, fmt.Errorf("clear %s: %w", ref.constraint, err)
			}
			total += n
			if n < batch {
				break
			}
		}
		if err := validateRef(ctx, client, "outbound_messages", ref.constraint); err != nil {
			return total, err
		}
	}
	return total, nil
}

// validateRef validates one key of table that a migration added NOT VALID. The table and the
// constraint are constants of the jobs package, never input.
func validateRef(ctx context.Context, client *ent.Client, table, constraint string) error {
	// raw sql: reads pg_constraint to see whether the migration's key is still NOT VALID; the
	// catalog is not an ent entity.
	rows, err := client.QueryContext(ctx,
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
	// (ADR 0033); ent runs no DDL outside a migration. The name is a constant of this file.
	if _, err := client.ExecContext(ctx, `ALTER TABLE `+table+` VALIDATE CONSTRAINT `+constraint); err != nil {
		return fmt.Errorf("validate %s: %w", constraint, err)
	}
	return nil
}
