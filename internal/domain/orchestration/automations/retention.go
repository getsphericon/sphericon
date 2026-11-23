package automations

import (
	"context"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationskippedentry"
	"github.com/getsphericon/sphericon/internal/kernel/drain"
)

// PruneSkippedEntries deletes the skipped entries recorded more than retention before the Instance
// clock's now, in batches of batchSize, and returns how many it removed: they are analytical
// history, kept for the window of the analytical Events (ADR 0019). A retention of zero or less
// disables the prune. It spans every Workspace, so it takes the raw client.
func (m *Module) PruneSkippedEntries(ctx context.Context, client *ent.Client, retention time.Duration, batchSize int) (int64, error) {
	if retention <= 0 {
		return 0, nil
	}
	cutoff := m.now().Add(-retention)
	total, err := drain.Batches(batchSize, func() (int, error) {
		ids, err := client.AutomationSkippedEntry.Query().
			Where(automationskippedentry.CreatedAtLT(cutoff)).
			Order(automationskippedentry.ByCreatedAt()).
			Limit(batchSize).
			IDs(ctx)
		if err != nil {
			return 0, fmt.Errorf("select expired skipped entries batch: %w", err)
		}
		if len(ids) == 0 {
			return 0, nil
		}
		n, err := client.AutomationSkippedEntry.Delete().Where(automationskippedentry.IDIn(ids...)).Exec(ctx)
		if err != nil {
			return 0, fmt.Errorf("delete expired skipped entries batch: %w", err)
		}
		return n, nil
	})
	return int64(total), err
}
