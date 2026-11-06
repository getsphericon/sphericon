package auditentries

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/auditentry"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseAuditEntriesIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.AuditEntry().Delete().Where(auditentry.IDIn(ids...)).Exec(ctx)
	return err
}
