package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
)

// backfillVersionsInterval is how often the backfill looks for Automations with no version. Once
// the first run has finished there is nothing left to find: a new Automation gets its version 1
// when it is created, so the tick is one cheap query.
const (
	backfillVersionsInterval = time.Hour
	backfillVersionsBatch    = 500
)

// BackfillAutomationVersionsArgs is the periodic tick that moves the steps of an Automation made
// before versions existed into its version 1 and points its Enrollments at it (ADR 0049). The
// migration that adds the Enrollment's version reference only adds (ADR 0033); this job is the
// heavy data work, and the migration that drops automations.definition refuses to run until it
// has finished.
type BackfillAutomationVersionsArgs struct{}

func (BackfillAutomationVersionsArgs) Kind() string { return "backfill_automation_versions" }

// BackfillAutomationVersionsWorker runs BackfillAutomationVersions. It reads every Workspace's
// rows, so it holds the raw ent client.
type BackfillAutomationVersionsWorker struct {
	river.WorkerDefaults[BackfillAutomationVersionsArgs]
	ent *ent.Client
}

func (w *BackfillAutomationVersionsWorker) Work(ctx context.Context, _ *river.Job[BackfillAutomationVersionsArgs]) error {
	made, pointed, err := BackfillAutomationVersions(ctx, w.ent, backfillVersionsBatch)
	if made > 0 || pointed > 0 {
		slog.InfoContext(ctx, "automation versions backfilled", "versions", made, "enrollments", pointed)
	}
	return err
}

// BackfillAutomationVersions gives every Automation with no version a version 1 holding its stored
// steps, then points every Enrollment with no version at its Automation's version 1. Each statement
// handles at most batch rows and the loops repeat until nothing is left, so a large table never
// holds a long lock, and a run interrupted halfway finishes on the next (every statement only
// touches what is still empty). It returns how many versions it made and Enrollments it moved.
//
// The steps are read from the definition column as JSON (to_jsonb), not by name: the migration
// that drops the column runs only once every Automation has a version, and after it this job must
// still plan, finding nothing.
func BackfillAutomationVersions(ctx context.Context, client *ent.Client, batch int) (made, pointed int, err error) {
	for {
		n, err := affected(client.ExecContext(ctx, `
INSERT INTO automation_versions (workspace_id, automation_id, version, steps)
SELECT a.workspace_id, a.id, 1, COALESCE(to_jsonb(a) ->> 'definition', '[]')
FROM automations a
WHERE NOT EXISTS (SELECT 1 FROM automation_versions v WHERE v.automation_id = a.id)
ORDER BY a.id
LIMIT $1`, batch))
		if err != nil {
			return made, pointed, fmt.Errorf("create version 1: %w", err)
		}
		made += n
		if n < batch {
			break
		}
	}
	for {
		n, err := affected(client.ExecContext(ctx, `
UPDATE automation_runs r SET version_id = v.id
FROM automation_versions v
WHERE v.automation_id = r.automation_id AND v.version = 1
  AND r.id IN (SELECT id FROM automation_runs WHERE version_id IS NULL ORDER BY id LIMIT $1)`, batch))
		if err != nil {
			return made, pointed, fmt.Errorf("point Enrollments at version 1: %w", err)
		}
		pointed += n
		if n < batch {
			break
		}
	}
	return made, pointed, nil
}

// affected is the row count of an executed statement.
func affected(res interface{ RowsAffected() (int64, error) }, err error) (int, error) {
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}
