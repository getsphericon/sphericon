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
	clearContactRefsInterval = time.Hour
	clearContactRefsBatch    = 1000
)

// ClearContactRefsArgs is the periodic tick that settles the rows whose contact_id points at a
// Contact that is gone (an Erasure from before the references were foreign keys), then validates
// the keys the migration added NOT VALID (ADR 0033: the migration only adds, this job is the heavy
// data work). A new write cannot dangle, since the keys are enforced from the migration on, so
// once a key has been validated every tick finds nothing.
type ClearContactRefsArgs struct{}

func (ClearContactRefsArgs) Kind() string { return "clear_contact_refs" }

// ClearContactRefsWorker runs ClearContactRefs. It reads every Workspace's rows, so it holds the
// raw ent client.
type ClearContactRefsWorker struct {
	river.WorkerDefaults[ClearContactRefsArgs]
	ent *ent.Client
}

func (w *ClearContactRefsWorker) Work(ctx context.Context, _ *river.Job[ClearContactRefsArgs]) error {
	settled, err := ClearContactRefs(ctx, w.ent, clearContactRefsBatch)
	if settled > 0 {
		slog.InfoContext(ctx, "dangling Contact references settled", "rows", settled)
	}
	return err
}

// contactRef is one reference to a Contact: its table, the key the migration added and the
// statement that settles the rows with no Contact behind them, the way the key would have: the
// row goes (it died with the Contact) or only the id goes (the row outlives it). The statements
// are literal, one per table, so nothing is spliced into SQL.
type contactRef struct {
	table      string
	constraint string
	// raw sql: a batched DELETE or UPDATE ... WHERE NOT EXISTS over contacts; ent writes do not
	// anti-join.
	settle string
}

var contactRefs = []contactRef{
	{"automation_runs", "automation_runs_contacts_contact", `
DELETE FROM automation_runs WHERE id IN (
  SELECT r.id FROM automation_runs r
  WHERE NOT EXISTS (SELECT 1 FROM contacts c WHERE c.id = r.contact_id)
  LIMIT $1)`},
	{"confirmations", "confirmations_contacts_contact", `
DELETE FROM confirmations WHERE id IN (
  SELECT r.id FROM confirmations r
  WHERE r.contact_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM contacts c WHERE c.id = r.contact_id)
  LIMIT $1)`},
	{"unsubscribes", "unsubscribes_contacts_contact", `
UPDATE unsubscribes SET contact_id = NULL WHERE id IN (
  SELECT r.id FROM unsubscribes r
  WHERE r.contact_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM contacts c WHERE c.id = r.contact_id)
  LIMIT $1)`},
	{"suppressions", "suppressions_contacts_contact", `
UPDATE suppressions SET contact_id = NULL WHERE id IN (
  SELECT r.id FROM suppressions r
  WHERE r.contact_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM contacts c WHERE c.id = r.contact_id)
  LIMIT $1)`},
	{"broadcast_recipients", "broadcast_recipients_contacts_contact", `
UPDATE broadcast_recipients SET contact_id = NULL WHERE id IN (
  SELECT r.id FROM broadcast_recipients r
  WHERE r.contact_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM contacts c WHERE c.id = r.contact_id)
  LIMIT $1)`},
	{"outbound_messages", "outbound_messages_contacts_contact", `
UPDATE outbound_messages SET contact_id = NULL WHERE id IN (
  SELECT r.id FROM outbound_messages r
  WHERE r.contact_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM contacts c WHERE c.id = r.contact_id)
  LIMIT $1)`},
}

// ClearContactRefs settles, table by table, the rows that name a Contact which is gone (at most
// batch rows per statement, so no long lock; a run interrupted halfway finishes on the next), and
// validates each key that is still NOT VALID once its table is clean. It returns how many rows it
// settled.
func ClearContactRefs(ctx context.Context, client *ent.Client, batch int) (int, error) {
	total := 0
	for _, ref := range contactRefs {
		for {
			// raw sql: the batched anti-join statement of contactRefs, one literal per table.
			n, err := affected(client.ExecContext(ctx, ref.settle, batch))
			if err != nil {
				return total, fmt.Errorf("settle %s: %w", ref.constraint, err)
			}
			total += n
			if n < batch {
				break
			}
		}
		if err := validateRef(ctx, client, ref.table, ref.constraint); err != nil {
			return total, err
		}
	}
	return total, nil
}
