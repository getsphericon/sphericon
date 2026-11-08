package events

import (
	"context"
	"errors"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/eventjournalentry"
)

// EraseWorkspaceJournalBatch removes a bounded batch through the journal's owner in its own
// transaction. Workspace erasure drains these before its final tombstone transaction, whose
// PurgeWorkspaceJobs fences publishers and deletes any tail that committed during the drain.
// Instance history belongs to no Workspace and is never selected here.
func (b *Bus) EraseWorkspaceJournalBatch(ctx context.Context, s *ent.Scoped, limit int) (int, error) {
	if limit < 1 {
		return 0, errors.New("events: journal erase batch must be positive")
	}
	removed := 0
	err := b.WithinTx(ctx, func(client *ent.Client, _ Publisher) error {
		ids, err := client.EventJournalEntry.Query().Where(eventjournalentry.WorkspaceID(s.WorkspaceID())).
			Order(eventjournalentry.ByID()).Limit(limit).IDs(ctx)
		if err != nil || len(ids) == 0 {
			return err
		}
		removed, err = client.EventJournalEntry.Delete().Where(eventjournalentry.WorkspaceID(s.WorkspaceID()), eventjournalentry.IDIn(ids...)).Exec(ctx)
		return err
	})
	if err != nil {
		return 0, err
	}
	return removed, nil
}
